package controllers

import (
	"context"
	"errors"
	"fmt"
	"github.com/hashicorp/go-uuid"
	"github.com/hibiken/asynq"
	"github.com/mylxsw/aidea-chat-server/api/auth"
	"github.com/mylxsw/aidea-chat-server/config"
	"github.com/mylxsw/aidea-chat-server/internal/coins"
	"github.com/mylxsw/aidea-chat-server/internal/consumer/tasks"
	"github.com/mylxsw/aidea-chat-server/internal/queue"
	"github.com/mylxsw/aidea-chat-server/pkg/misc"
	"github.com/mylxsw/aidea-chat-server/pkg/rate"
	"github.com/mylxsw/aidea-chat-server/pkg/repo"
	"github.com/mylxsw/aidea-chat-server/pkg/repo/model"
	"github.com/mylxsw/aidea-chat-server/pkg/token"
	"github.com/mylxsw/aidea-chat-server/pkg/wechat"
	"github.com/mylxsw/asteria/log"
	"github.com/mylxsw/glacier/infra"
	"github.com/mylxsw/glacier/web"
	"github.com/mylxsw/go-utils/ternary"
	"github.com/redis/go-redis/v9"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/Timothylock/go-signin-with-apple/apple"
	passwordvalidator "github.com/wagslane/go-password-validator"
)

type AuthController struct {
	conf    *config.Config `autowire:"@"`
	queue   *queue.Queue   `autowire:"@"`
	limiter *rate.Limiter  `autowire:"@"`
	tk      *token.Token   `autowire:"@"`
	rds     *redis.Client  `autowire:"@"`
	wc      *wechat.WeChat `autowire:"@"`

	repo *repo.Repository `autowire:"@"`
}

func NewAuthController(resolver infra.Resolver) web.Controller {
	ctl := AuthController{}
	resolver.MustAutoWire(&ctl)
	return &ctl
}

func (ctl *AuthController) Register(router web.Router) {
	router.Group("/auth", func(router web.Router) {
		// 登录
		router.Post("/sign-in", ctl.SignInWithPassword)
		router.Post("/sign-in/sms-code", ctl.SendSigninSMSCode)
		router.Post("/sign-in/email-code", ctl.SendEmailCode)

		// Apple 登录
		router.Post("/sign-in-apple", ctl.SignInWithApple)

		// 微信登录
		router.Post("/sign-in-wechat/try", ctl.TrySignInWithWechat)
		router.Post("/sign-in-wechat", ctl.SignInWithWechat)
		router.Post("/bind-wechat", ctl.BindWeChat)

		// 注册登录二合一
		router.Post("/2in1/check", ctl.CheckPhoneExistence)
		router.Post("/2in1/sign-inup", ctl.SignInOrUpWithSMSCode)

		// 注册
		router.Post("/sign-up", ctl.SignUpWithPassword)
		router.Post("/sign-up/email-code", ctl.SignUpSendEmailCode)
		router.Post("/sign-up/sms-code", ctl.BindPhoneSendSMSCode)

		// 找回密码
		router.Post("/reset-password/email-code", ctl.SendEmailCode)
		router.Post("/reset-password/sms-code", ctl.ResetPasswordSMSCode)
		router.Post("/reset-password", ctl.ResetPassword)

		// 绑定手机号
		router.Post("/bind-phone/sms-code", ctl.BindPhoneSendSMSCode)
		router.Post("/bind-phone", ctl.BindPhone)
	})
}

// minEntropyBits is the minimum entropy bits required for a password to be considered strong enough.
const minEntropyBits = 40

func (ctl *AuthController) CheckPhoneExistence(ctx context.Context, webCtx web.Context) web.Response {
	username := strings.TrimSpace(webCtx.Input("username"))
	if username == "" {
		return webCtx.JSONError("username is required", http.StatusBadRequest)
	}

	if !misc.IsPhoneNumber(username) && !misc.IsEmail(username) {
		return webCtx.JSONError("invalid account format. It must be a mobile phone number or email address", http.StatusBadRequest)
	}

	// 检查用户是否存在
	var user *model.Users
	var err error
	var signInMethod string
	if misc.IsPhoneNumber(username) {
		user, err = ctl.repo.User.GetUserByPhone(ctx, username)
		signInMethod = "sms_code"
	} else {
		user, err = ctl.repo.User.GetUserByEmail(ctx, username)
		signInMethod = "email_code"
	}
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return webCtx.JSON(web.M{"exist": false, "sign_in_method": signInMethod})
		}

		if errors.Is(err, repo.ErrUserAccountDisabled) {
			return webCtx.JSONError("account unavailable: User account has been destroyed", http.StatusForbidden)
		}

		log.WithFields(log.Fields{
			"username": username,
		}).Errorf("failed to get user: %s", err)

		return webCtx.JSONError("internal server error", http.StatusInternalServerError)
	}

	if user.PreferSigninMethod == "" {
		if user.Password != "" {
			user.PreferSigninMethod = "password"
		} else {
			user.PreferSigninMethod = signInMethod
		}
	}

	return webCtx.JSON(web.M{"exist": true, "sign_in_method": user.PreferSigninMethod})
}

func (ctl *AuthController) SignInOrUpWithSMSCode(ctx context.Context, webCtx web.Context) web.Response {
	username := strings.TrimSpace(webCtx.Input("username"))
	if username == "" {
		return webCtx.JSONError("账号不能为空", http.StatusBadRequest)
	}

	if !misc.IsPhoneNumber(username) && !misc.IsEmail(username) {
		return webCtx.JSONError("账号格式错误，必须为手机号码或者邮箱", http.StatusBadRequest)
	}

	inviteCode := strings.TrimSpace(webCtx.Input("invite_code"))
	if inviteCode != "" {
		if err := ctl.verifyInviteCode(ctx, inviteCode); err != nil {
			if errors.Is(err, repo.ErrNotFound) {
				return webCtx.JSONError("邀请码无效", http.StatusBadRequest)
			}

			log.WithFields(log.Fields{
				"invite_code": inviteCode,
			}).Errorf("failed to verify invite code: %s", err)

			return webCtx.JSONError(InternalServerError, http.StatusInternalServerError)
		}
	}

	verifyCodeId := strings.TrimSpace(webCtx.Input("verify_code_id"))
	if verifyCodeId == "" {
		return webCtx.JSONError("验证码 ID 不能为空", http.StatusBadRequest)
	}

	verifyCode := strings.TrimSpace(webCtx.Input("verify_code"))
	if verifyCode == "" {
		return webCtx.JSONError("验证码不能为空", http.StatusBadRequest)
	}

	// 检查验证码是否正确
	realVerifyCode, err := ctl.rds.Get(ctx, fmt.Sprintf("auth:verify-code:%s:%s", verifyCodeId, username)).Result()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			log.WithFields(log.Fields{
				"username": username,
				"id":       verifyCodeId,
				"code":     verifyCode,
			}).Errorf("failed to get email code: %s", err)
		}
		return webCtx.JSONError("验证码已过期，请重新获取", http.StatusBadRequest)
	}

	if realVerifyCode != verifyCode {
		return webCtx.JSONError("验证码错误", http.StatusBadRequest)
	}

	_ = ctl.rds.Del(ctx, fmt.Sprintf("auth:verify-code:%s:%s", verifyCodeId, username)).Err()

	// 检查用户信息
	var user *model.Users
	if misc.IsPhoneNumber(username) {
		user, err = ctl.repo.User.GetUserByPhone(ctx, username)
	} else {
		user, err = ctl.repo.User.GetUserByEmail(ctx, username)
	}
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			// 用户不存在，注册新用户
			return ctl.createAccount(ctx, webCtx, username, "", inviteCode)
		}

		log.WithFields(log.Fields{
			"username": username,
		}).Errorf("failed to get user: %s", err)
		return webCtx.JSONError(InternalServerError, http.StatusInternalServerError)
	}

	// 微信绑定 token
	wechatBindToken := strings.TrimSpace(webCtx.Input("wechat_bind_token"))
	if wechatBindToken != "" {
		if err := ctl.bindWeChatWithToken(ctx, user.Id, wechatBindToken); err != nil {
			log.WithFields(log.Fields{
				"username": username,
				"token":    wechatBindToken,
			}).Errorf("failed to bind wechat: %s", err)
		}
	}

	return webCtx.JSON(buildUserLoginRes(user, false, ctl.tk))
}

// bindWeChatWithToken 绑定微信
func (ctl *AuthController) bindWeChatWithToken(ctx context.Context, userID int64, tokenValue string) error {
	payload, err := ctl.tk.ParseToken(tokenValue)
	if err != nil {
		return errors.New("invalid token")
	}

	unionID := payload.StringValue("union_id")
	nickname := payload.StringValue("nickname")
	avatar := payload.StringValue("avatar")

	if unionID == "" {
		return errors.New("invalid token")
	}

	return ctl.repo.User.BindWeChat(ctx, userID, unionID, nickname, avatar)
}

func (ctl *AuthController) SendSigninSMSCode(ctx context.Context, webCtx web.Context) web.Response {
	return ctl.sendSMSCode(ctx, webCtx, func(username string) web.Response {
		// 检查用户是否存在
		if _, err := ctl.repo.User.GetUserByPhone(ctx, username); err != nil {
			if errors.Is(err, repo.ErrNotFound) {
				return webCtx.JSONError("用户不存在", http.StatusBadRequest)
			}

			log.WithFields(log.Fields{
				"username": username,
			}).Errorf("failed to get user: %s", err)

			return webCtx.JSONError(InternalServerError, http.StatusInternalServerError)
		}

		return nil
	})
}

// verifyInviteCode 验证邀请码
func (ctl *AuthController) verifyInviteCode(ctx context.Context, code string) error {
	_, err := ctl.repo.User.GetUserByInviteCode(ctx, code)
	if errors.Is(err, repo.ErrUserAccountDisabled) {
		return repo.ErrNotFound
	}

	return err
}

// BindPhone 绑定手机号码
func (ctl *AuthController) BindPhone(ctx context.Context, webCtx web.Context, current *auth.User) web.Response {
	username := strings.TrimSpace(webCtx.Input("username"))
	if username == "" {
		return webCtx.JSONError("手机号不能为空", http.StatusBadRequest)
	}

	if !misc.IsPhoneNumber(username) {
		return webCtx.JSONError("手机号格式错误", http.StatusBadRequest)
	}

	inviteCode := strings.TrimSpace(webCtx.Input("invite_code"))
	if inviteCode != "" {
		if err := ctl.verifyInviteCode(ctx, inviteCode); err != nil {
			if errors.Is(err, repo.ErrNotFound) {
				return webCtx.JSONError("邀请码无效", http.StatusBadRequest)
			}

			log.WithFields(log.Fields{
				"invite_code": inviteCode,
			}).Errorf("failed to verify invite code: %s", err)

			return webCtx.JSONError(InternalServerError, http.StatusInternalServerError)
		}
	}

	verifyCodeId := strings.TrimSpace(webCtx.Input("verify_code_id"))
	if verifyCodeId == "" {
		return webCtx.JSONError("验证码 ID 不能为空", http.StatusBadRequest)
	}

	verifyCode := strings.TrimSpace(webCtx.Input("verify_code"))
	if verifyCode == "" {
		return webCtx.JSONError("验证码不能为空", http.StatusBadRequest)
	}

	// 检查验证码是否正确
	realVerifyCode, err := ctl.rds.Get(ctx, fmt.Sprintf("auth:verify-code:%s:%s", verifyCodeId, username)).Result()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			log.WithFields(log.Fields{
				"username": username,
				"id":       verifyCodeId,
				"code":     verifyCode,
			}).Errorf("failed to get email code: %s", err)
		}
		return webCtx.JSONError("验证码已过期，请重新获取", http.StatusBadRequest)
	}

	if realVerifyCode != verifyCode {
		return webCtx.JSONError("验证码错误", http.StatusBadRequest)
	}

	_ = ctl.rds.Del(ctx, fmt.Sprintf("auth:verify-code:%s:%s", verifyCodeId, username)).Err()

	// 检查用户信息
	user, err := ctl.repo.User.GetUserByID(ctx, current.ID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return webCtx.JSONError("用户不存在", http.StatusBadRequest)
		}

		log.WithFields(log.Fields{
			"user_id": current.ID,
		}).Errorf("failed to get user: %s", err)
		return webCtx.JSONError("内部错误，请稍后再试", http.StatusInternalServerError)
	}

	if user.Phone != "" {
		if user.Phone == username {
			return webCtx.JSON(buildUserLoginRes(user, false, ctl.tk))
		}

		return webCtx.JSONError("绑定失败，已绑定其它手机号", http.StatusBadRequest)
	}

	// 检查手机号是否绑定到其它账号
	if u, err := ctl.repo.User.GetUserByPhone(ctx, username); err != nil {
		if !errors.Is(err, repo.ErrNotFound) {
			log.WithFields(log.Fields{
				"username": username,
			}).Errorf("failed to get user: %s", err)

			return webCtx.JSONError(InternalServerError, http.StatusInternalServerError)
		}
	} else {
		if u != nil {
			return webCtx.JSONError("手机号已绑定其它账号", http.StatusBadRequest)
		}
	}

	// 之前绑定过手机，不再支持邀请码
	isNewUser := true
	if user.Phone != "" {
		inviteCode = ""
		isNewUser = false
	}

	// 绑定手机号码，如果之前的手机号码为空，则认为是初始绑定，发送绑定事件，用于赠送初始智慧果
	eventID, err := ctl.repo.User.BindPhone(ctx, user.Id, username, user.Phone == "")
	if err != nil {
		log.WithFields(log.Fields{
			"user_id":  current.ID,
			"username": username,
		}).Errorf("failed to update phone: %s", err)
		return webCtx.JSONError("内部错误，请稍后再试", http.StatusInternalServerError)
	}

	// 绑定手机成功后，需要清空当前用户的缓存 service.GetUserByID
	_ = ctl.rds.Del(ctx, fmt.Sprintf("user:%d:info", current.ID)).Err()

	if eventID > 0 {
		payload := tasks.BindPhonePayload{
			UserID:     user.Id,
			Phone:      username,
			EventID:    eventID,
			InviteCode: inviteCode,
			CreatedAt:  time.Now(),
		}

		if _, err := ctl.queue.Enqueue(ctx, &payload, asynq.Queue("user")); err != nil {
			log.WithFields(log.Fields{
				"user_id":  user.Id,
				"username": username,
				"event_id": eventID,
			}).Errorf("failed to enqueue bind phone task: %s", err)
		}
	}

	user.Phone = username

	return webCtx.JSON(buildUserLoginRes(user, isNewUser, ctl.tk))
}

func (ctl *AuthController) ResetPassword(ctx context.Context, webCtx web.Context, userRepo *repo.UserRepo, rds *redis.Client) web.Response {
	username := strings.TrimSpace(webCtx.Input("username"))
	if username == "" {
		return webCtx.JSONError("用户名不能为空", http.StatusBadRequest)
	}

	if !misc.IsEmail(username) && !misc.IsPhoneNumber(username) {
		return webCtx.JSONError("用户名格式错误", http.StatusBadRequest)
	}

	password := strings.TrimSpace(webCtx.Input("password"))
	if len(password) < 8 || len(password) > 20 {
		return webCtx.JSONError("密码长度必须在 8-20 位之间", http.StatusBadRequest)
	}

	if err := passwordvalidator.Validate(password, minEntropyBits); err != nil {
		return webCtx.JSONError("密码强度不够，建议使用字母、数字、特殊符号组合", http.StatusBadRequest)
	}

	verifyCodeId := strings.TrimSpace(webCtx.Input("verify_code_id"))
	if verifyCodeId == "" {
		return webCtx.JSONError("验证码 ID 不能为空", http.StatusBadRequest)
	}

	verifyCode := strings.TrimSpace(webCtx.Input("verify_code"))
	if verifyCode == "" {
		return webCtx.JSONError("验证码不能为空", http.StatusBadRequest)
	}

	// 流控：每个用户每 60 分钟只能重置密码 5 次
	err := ctl.limiter.Allow(ctx, fmt.Sprintf("auth:reset-password:%s:limit", username), rate.MaxRequestsInPeriod(5, 60*time.Minute))
	if err != nil {
		if errors.Is(err, rate.ErrRateLimitExceeded) {
			return webCtx.JSONError("操作频率过高，请稍后再试", http.StatusTooManyRequests)
		}

		log.WithFields(log.Fields{
			"username": username,
		}).Errorf("failed to check verify code rate limit: %s", err)
		return webCtx.JSONError("内部错误，请稍后再试", http.StatusInternalServerError)
	}

	// 检查验证码是否正确
	realVerifyCode, err := rds.Get(ctx, fmt.Sprintf("auth:verify-code:%s:%s", verifyCodeId, username)).Result()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			log.WithFields(log.Fields{
				"username": username,
				"id":       verifyCodeId,
				"code":     verifyCode,
			}).Errorf("failed to get verify code: %s", err)
		}
		return webCtx.JSONError("验证码已过期，请重新获取", http.StatusBadRequest)
	}

	if realVerifyCode != verifyCode {
		return webCtx.JSONError("验证码错误", http.StatusBadRequest)
	}

	_ = rds.Del(ctx, fmt.Sprintf("auth:verify-code:%s:%s", verifyCodeId, username)).Err()

	var user *model.Users
	if misc.IsEmail(username) {
		user, err = userRepo.GetUserByEmail(ctx, username)
		if err != nil {
			log.WithFields(log.Fields{
				"username": username,
			}).Errorf("failed to get user: %s", err)
			return webCtx.JSONError("用户不存在", http.StatusBadRequest)
		}
	} else {
		user, err = userRepo.GetUserByPhone(ctx, username)
		if err != nil {
			log.WithFields(log.Fields{
				"username": username,
			}).Errorf("failed to get user: %s", err)
			return webCtx.JSONError("用户不存在", http.StatusBadRequest)
		}
	}

	if err := userRepo.UpdatePassword(ctx, user.Id, password); err != nil {
		log.WithFields(log.Fields{
			"username": username,
		}).Errorf("failed to update password: %s", err)
		return webCtx.JSONError(InternalServerError, http.StatusInternalServerError)
	}

	return webCtx.JSON(web.M{})
}

// SendEmailCode 发送邮件验证码
func (ctl *AuthController) SendEmailCode(ctx context.Context, webCtx web.Context, userRepo *repo.UserRepo, rds *redis.Client) web.Response {
	username := strings.TrimSpace(webCtx.Input("username"))
	if username == "" {
		return webCtx.JSONError("用户名不能为空", http.StatusBadRequest)
	}

	if !misc.IsEmail(username) {
		return webCtx.JSONError("用户名格式错误", http.StatusBadRequest)
	}

	// 流控：每个用户每分钟只能发送一次邮件
	mailCodeRateLimitPerMinKey := fmt.Sprintf("auth:email-code:limit:%s", username)
	optCount, err := ctl.limiter.OperationCount(ctx, mailCodeRateLimitPerMinKey)
	if err != nil {
		log.WithFields(log.Fields{
			"username": username,
		}).Errorf("failed to check email code rate limit: %s", err)
		return webCtx.JSONError(InternalServerError, http.StatusInternalServerError)
	}

	if optCount > 0 {
		return webCtx.JSONError("发送邮件过于频繁，请稍后再试", http.StatusTooManyRequests)
	}

	// 流控：每个用户每小时只能发送 5 次邮件
	if err := ctl.limiter.Allow(ctx, fmt.Sprintf("auth:email-code:limit:%s:retrive-pwd", username), rate.MaxRequestsInPeriod(5, time.Hour)); err != nil {
		if errors.Is(err, rate.ErrRateLimitExceeded) {
			return webCtx.JSONError("操作频率过高，请稍后再试", http.StatusTooManyRequests)
		}
		log.WithFields(log.Fields{
			"username": username,
		}).Errorf("failed to check email code rate limit: %s", err)
		return webCtx.JSONError(InternalServerError, http.StatusInternalServerError)
	}

	// 检查用户是否存在
	if _, err := userRepo.GetUserByEmail(ctx, username); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return webCtx.JSONError("用户不存在", http.StatusBadRequest)
		}

		log.WithFields(log.Fields{
			"username": username,
		}).Errorf("failed to get user: %s", err)

		return webCtx.JSONError(InternalServerError, http.StatusInternalServerError)
	}

	// 生成验证码
	id, _ := uuid.GenerateUUID()
	code := verifyCodeGenerator()

	// 发送邮件
	log.WithFields(log.Fields{
		"username": username,
		"id":       id,
		"code":     code,
	}).Debugf("send email code: %s", code)

	mailPayload := tasks.MailPayload{
		To:        []string{username},
		Subject:   "验证码",
		Body:      fmt.Sprintf("您的验证码是：%s， 请在 %s 之前使用。", code, time.Now().Add(10*time.Minute).Format("2006-01-02 15:04:05")),
		CreatedAt: time.Now(),
	}

	taskId, err := ctl.queue.Enqueue(ctx, &mailPayload, asynq.Queue("mail"))
	if err != nil {
		log.WithFields(log.Fields{
			"username": username,
			"id":       id,
			"code":     code,
		}).Errorf("failed to enqueue mail task: %s", err)
		return webCtx.JSONError(InternalServerError, http.StatusInternalServerError)
	}

	if err := rds.SetNX(ctx, fmt.Sprintf("auth:verify-code:%s:%s", id, username), code, 15*time.Minute).Err(); err != nil {
		log.WithFields(log.Fields{
			"username": username,
			"id":       id,
			"code":     code,
			"task_id":  taskId,
		}).Errorf("failed to set email code: %s", err)
		return webCtx.JSONError(InternalServerError, http.StatusInternalServerError)
	}

	// 设置流控
	if err := ctl.limiter.OperationIncr(ctx, mailCodeRateLimitPerMinKey, 50*time.Second); err != nil {
		log.WithFields(log.Fields{
			"username": username,
			"id":       id,
			"code":     code,
			"task_id":  taskId,
		}).Errorf("failed to set email code rate limit: %s", err)
	}

	return webCtx.JSON(web.M{
		"id": id,
	})
}

func (ctl *AuthController) sendSMSCode(ctx context.Context, webCtx web.Context, cb func(phone string) web.Response) web.Response {
	username := strings.TrimSpace(webCtx.Input("username"))
	if username == "" {
		return webCtx.JSONError("手机号不能为空", http.StatusBadRequest)
	}

	if !misc.IsPhoneNumber(username) {
		return webCtx.JSONError("手机号格式错误", http.StatusBadRequest)
	}

	// 流控：每个用户每分钟只能发送一次短信
	smsCodeRateLimitPerMinute := fmt.Sprintf("auth:sms-code:limit:%s:min", username)
	optCountPerMin, err := ctl.limiter.OperationCount(ctx, smsCodeRateLimitPerMinute)
	if err != nil {
		log.WithFields(log.Fields{
			"username": username,
		}).Errorf("failed to check sms code rate limit: %s", err)
		return webCtx.JSONError(InternalServerError, http.StatusInternalServerError)
	}

	if optCountPerMin > 0 {
		return webCtx.JSONError("发送短信验证码过于频繁，请稍后再试", http.StatusTooManyRequests)
	}

	// 流控：每个用户每天只能发送 5 次短信
	smsCodeRateLimitPerDay := fmt.Sprintf("auth:sms-code:limit:%s:day", username)
	optCountPerDay, err := ctl.limiter.OperationCount(ctx, smsCodeRateLimitPerDay)
	if err != nil {
		log.WithFields(log.Fields{
			"username": username,
		}).Errorf("failed to check sms code rate limit: %s", err)
		return webCtx.JSONError(InternalServerError, http.StatusInternalServerError)
	}

	if optCountPerDay >= 5 {
		return webCtx.JSONError("当前账号今日发送验证码次数已达上限，请 24 小时后再试", http.StatusTooManyRequests)
	}

	// 业务检查
	if rs := cb(username); rs != nil {
		return rs
	}

	// 生成验证码
	id, _ := uuid.GenerateUUID()
	code := verifyCodeGenerator()

	smsPayload := tasks.SMSVerifyCodePayload{
		Receiver:  username,
		Code:      code,
		CreatedAt: time.Now(),
	}

	taskId, err := ctl.queue.Enqueue(ctx, &smsPayload, asynq.Queue("mail"))
	if err != nil {
		log.WithFields(log.Fields{
			"username": username,
			"id":       id,
			"code":     code,
		}).Errorf("failed to enqueue mail task: %s", err)
		return webCtx.JSONError(InternalServerError, http.StatusInternalServerError)
	}

	if err := ctl.rds.SetNX(ctx, fmt.Sprintf("auth:verify-code:%s:%s", id, username), code, 15*time.Minute).Err(); err != nil {
		log.WithFields(log.Fields{
			"username": username,
			"id":       id,
			"code":     code,
			"task_id":  taskId,
		}).Errorf("failed to set email code: %s", err)
		return webCtx.JSONError(InternalServerError, http.StatusInternalServerError)
	}

	// 设置流控
	if err := ctl.limiter.OperationIncr(ctx, smsCodeRateLimitPerMinute, 50*time.Second); err != nil {
		log.WithFields(log.Fields{
			"username": username,
			"id":       id,
			"code":     code,
			"task_id":  taskId,
		}).Errorf("failed to set email code rate limit: %s", err)
	}

	if err := ctl.limiter.OperationIncr(ctx, smsCodeRateLimitPerDay, 24*time.Hour); err != nil {
		log.WithFields(log.Fields{
			"username": username,
			"id":       id,
			"code":     code,
			"task_id":  taskId,
		}).Errorf("failed to set email code rate limit: %s", err)
	}

	return webCtx.JSON(web.M{
		"id": id,
	})
}

// ResetPasswordSMSCode 发送找回密码短信验证码
func (ctl *AuthController) ResetPasswordSMSCode(ctx context.Context, webCtx web.Context) web.Response {
	return ctl.sendSMSCode(ctx, webCtx, func(username string) web.Response {
		// 检查用户是否存在
		if _, err := ctl.repo.User.GetUserByPhone(ctx, username); err != nil {
			if err == repo.ErrNotFound {
				return webCtx.JSONError("用户不存在", http.StatusBadRequest)
			}

			log.WithFields(log.Fields{
				"username": username,
			}).Errorf("failed to get user: %s", err)

			return webCtx.JSONError(InternalServerError, http.StatusInternalServerError)
		}

		return nil
	})
}

func (ctl *AuthController) BindPhoneSendSMSCode(ctx context.Context, webCtx web.Context) web.Response {
	return ctl.sendSMSCode(ctx, webCtx, func(username string) web.Response {
		// 检查用户是否存在
		if u, err := ctl.repo.User.GetUserByPhone(ctx, username); err != nil {
			if err != repo.ErrNotFound {
				log.WithFields(log.Fields{
					"username": username,
				}).Errorf("failed to get user: %s", err)

				return webCtx.JSONError(InternalServerError, http.StatusInternalServerError)
			}
		} else {
			if u != nil {
				return webCtx.JSONError("手机号已绑定，可直接登录", http.StatusBadRequest)
			}
		}

		return nil
	})
}

// SignUpSendEmailCode 发送注册邮件验证码
func (ctl *AuthController) SignUpSendEmailCode(ctx context.Context, webCtx web.Context) web.Response {
	username := strings.TrimSpace(webCtx.Input("username"))
	if username == "" {
		return webCtx.JSONError("用户名不能为空", http.StatusBadRequest)
	}

	if !misc.IsEmail(username) {
		return webCtx.JSONError("用户名格式错误", http.StatusBadRequest)
	}

	// 流控：每个用户每分钟只能发送一次邮件
	mailCodeRateLimitKey := fmt.Sprintf("auth:email-code:limit:%s", username)
	optCount, err := ctl.limiter.OperationCount(ctx, mailCodeRateLimitKey)
	if err != nil {
		log.WithFields(log.Fields{
			"username": username,
		}).Errorf("failed to check email code rate limit: %s", err)
		return webCtx.JSONError(InternalServerError, http.StatusInternalServerError)
	}

	if optCount > 0 {
		return webCtx.JSONError("发送邮件过于频繁，请稍后再试", http.StatusTooManyRequests)
	}

	// 检查用户是否存在
	if u, err := ctl.repo.User.GetUserByEmail(ctx, username); err != nil {
		if !errors.Is(err, repo.ErrNotFound) {
			log.WithFields(log.Fields{
				"username": username,
			}).Errorf("failed to get user: %s", err)

			return webCtx.JSONError(InternalServerError, http.StatusInternalServerError)
		}
	} else {
		if u != nil {
			return webCtx.JSONError("该账号已被注册，请登录", http.StatusBadRequest)
		}
	}

	// 生成验证码
	id, _ := uuid.GenerateUUID()
	code := verifyCodeGenerator()

	// 发送邮件
	log.WithFields(log.Fields{
		"username": username,
		"id":       id,
		"code":     code,
	}).Debugf("send email code: %s", code)

	mailPayload := tasks.MailPayload{
		To:        []string{username},
		Subject:   "验证您的电子邮件地址",
		Body:      fmt.Sprintf("您的验证码是：%s， 请在 %s 之前使用。", code, time.Now().Add(10*time.Minute).Format("2006-01-02 15:04:05")),
		CreatedAt: time.Now(),
	}

	taskId, err := ctl.queue.Enqueue(ctx, &mailPayload, asynq.Queue("mail"))
	if err != nil {
		log.WithFields(log.Fields{
			"username": username,
			"id":       id,
			"code":     code,
		}).Errorf("failed to enqueue mail task: %s", err)
		return webCtx.JSONError(InternalServerError, http.StatusInternalServerError)
	}

	if err := ctl.rds.SetNX(ctx, fmt.Sprintf("auth:verify-code:%s:%s", id, username), code, 15*time.Minute).Err(); err != nil {
		log.WithFields(log.Fields{
			"username": username,
			"id":       id,
			"code":     code,
			"task_id":  taskId,
		}).Errorf("failed to set email code: %s", err)
		return webCtx.JSONError(InternalServerError, http.StatusInternalServerError)
	}

	// 设置流控
	if err := ctl.limiter.OperationIncr(ctx, mailCodeRateLimitKey, 50*time.Second); err != nil {
		log.WithFields(log.Fields{
			"username": username,
			"id":       id,
			"code":     code,
			"task_id":  taskId,
		}).Errorf("failed to set email code rate limit: %s", err)
	}

	return webCtx.JSON(web.M{
		"id": id,
	})
}

func verifyCodeGenerator() string {
	return fmt.Sprintf("%d", rand.Intn(900000)+100000)
}

// SignUpWithPassword 用户账号注册
func (ctl *AuthController) SignUpWithPassword(ctx context.Context, webCtx web.Context) web.Response {
	username := strings.TrimSpace(webCtx.Input("username"))
	if username == "" {
		return webCtx.JSONError("用户名不能为空", http.StatusBadRequest)
	}

	if !misc.IsEmail(username) && !misc.IsPhoneNumber(username) {
		return webCtx.JSONError("用户名格式错误", http.StatusBadRequest)
	}

	password := strings.TrimSpace(webCtx.Input("password"))
	if len(password) < 8 || len(password) > 20 {
		return webCtx.JSONError("密码长度必须在 8-20 位之间", http.StatusBadRequest)
	}

	if err := passwordvalidator.Validate(password, minEntropyBits); err != nil {
		return webCtx.JSONError("密码强度不够，建议使用字母、数字、特殊符号组合", http.StatusBadRequest)
	}

	inviteCode := strings.TrimSpace(webCtx.Input("invite_code"))
	if inviteCode != "" {
		if err := ctl.verifyInviteCode(ctx, inviteCode); err != nil {
			if errors.Is(err, repo.ErrNotFound) {
				return webCtx.JSONError("邀请码无效", http.StatusBadRequest)
			}

			log.WithFields(log.Fields{
				"invite_code": inviteCode,
			}).Errorf("failed to verify invite code: %s", err)

			return webCtx.JSONError(InternalServerError, http.StatusInternalServerError)
		}
	}

	verifyCodeId := strings.TrimSpace(webCtx.Input("verify_code_id"))
	if verifyCodeId == "" {
		return webCtx.JSONError("验证码 ID 不能为空", http.StatusBadRequest)
	}

	verifyCode := strings.TrimSpace(webCtx.Input("verify_code"))
	if verifyCode == "" {
		return webCtx.JSONError("验证码不能为空", http.StatusBadRequest)
	}

	realVerifyCode, err := ctl.rds.Get(ctx, fmt.Sprintf("auth:verify-code:%s:%s", verifyCodeId, username)).Result()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			log.WithFields(log.Fields{
				"username": username,
				"id":       verifyCodeId,
				"code":     verifyCode,
			}).Errorf("failed to get verify code: %s", err)
		}
		return webCtx.JSONError("验证码已过期，请重新获取", http.StatusBadRequest)
	}

	if realVerifyCode != verifyCode {
		return webCtx.JSONError("验证码错误", http.StatusBadRequest)
	}

	_ = ctl.rds.Del(ctx, fmt.Sprintf("auth:verify-code:%s:%s", verifyCodeId, username)).Err()

	return ctl.createAccount(ctx, webCtx, username, password, inviteCode)
}

// createAccount 创建账号
func (ctl *AuthController) createAccount(ctx context.Context, webCtx web.Context, username string, password string, inviteCode string) web.Response {
	realname := strings.TrimSpace(webCtx.Input("realname"))

	var user *model.Users
	var eventID int64
	var err error

	isEmailSignup := misc.IsEmail(username)
	if isEmailSignup {
		user, eventID, err = ctl.repo.User.SignUpEmail(ctx, username, password, realname)
		if err != nil {
			log.WithFields(log.Fields{
				"username": username,
				"realname": realname,
			}).Errorf("failed to sign up: %s", err)
			return webCtx.JSONError(err.Error(), http.StatusBadRequest)
		}

	} else {
		user, eventID, err = ctl.repo.User.SignUpPhone(ctx, username, password, realname)
		if err != nil {
			log.WithFields(log.Fields{
				"username": username,
				"realname": realname,
			}).Errorf("failed to sign up: %s", err)
			return webCtx.JSONError(err.Error(), http.StatusBadRequest)
		}
	}

	if eventID > 0 {
		payload := tasks.SignupPayload{
			UserID:     user.Id,
			InviteCode: inviteCode,
			EventID:    eventID,
			CreatedAt:  time.Now(),
		}

		if isEmailSignup {
			payload.Email = username
		} else {
			payload.Phone = username
		}

		if _, err := ctl.queue.Enqueue(ctx, &payload, asynq.Queue("user")); err != nil {
			log.WithFields(log.Fields{
				"username": username,
				"event_id": eventID,
			}).Errorf("failed to enqueue signup task: %s", err)
		}
	}

	// 微信绑定 token
	wechatBindToken := strings.TrimSpace(webCtx.Input("wechat_bind_token"))
	if wechatBindToken != "" {
		if err := ctl.bindWeChatWithToken(ctx, user.Id, wechatBindToken); err != nil {
			log.WithFields(log.Fields{
				"user_id": user.Id,
				"token":   wechatBindToken,
			}).Errorf("failed to bind wechat: %s", err)
		}
	}

	return webCtx.JSON(buildUserLoginRes(user, true, ctl.tk))
}

// SignInWithPassword 用户账号登录
func (ctl *AuthController) SignInWithPassword(ctx context.Context, webCtx web.Context) web.Response {
	username := webCtx.Input("username")
	password := webCtx.Input("password")

	if username == "" || password == "" {
		return webCtx.JSONError("用户名或密码不能为空", http.StatusBadRequest)
	}

	if err := ctl.limiter.Allow(ctx, fmt.Sprintf("auth:%s:login", username), rate.MaxRequestsInPeriod(5, 10*time.Minute)); err != nil {
		if errors.Is(err, rate.ErrRateLimitExceeded) {
			return webCtx.JSONError("登录频率过高，请稍后再试", http.StatusTooManyRequests)
		}

		log.WithFields(log.Fields{
			"username": username,
		}).Errorf("failed to check login rate: %s", err)
		return webCtx.JSONError(InternalServerError, http.StatusInternalServerError)
	}

	user, err := ctl.repo.User.SignInWithPassword(ctx, username, password)
	if err != nil {
		return webCtx.JSONError("用户名或密码错误", http.StatusBadRequest)
	}

	// 微信绑定 token
	wechatBindToken := strings.TrimSpace(webCtx.Input("wechat_bind_token"))
	if wechatBindToken != "" {
		if err := ctl.bindWeChatWithToken(ctx, user.Id, wechatBindToken); err != nil {
			log.WithFields(log.Fields{
				"user_id": user.Id,
				"token":   wechatBindToken,
			}).Errorf("failed to bind wechat: %s", err)
		}
	}

	return webCtx.JSON(buildUserLoginRes(user, false, ctl.tk))
}

// TrySignInWithWechat 尝试使用微信登录，返回微信端用户信息 token + 用户是否存在
func (ctl *AuthController) TrySignInWithWechat(ctx context.Context, webCtx web.Context) web.Response {
	code := strings.TrimSpace(webCtx.Input("code"))
	if code == "" {
		return webCtx.JSONError("code 不能为空", http.StatusBadRequest)
	}

	accessToken, err := ctl.wc.OAuthAccessToken(ctx, code)
	if err != nil {
		log.WithFields(log.Fields{"code": code}).Errorf("failed to get wechat access token: %s", err)
		return webCtx.JSONError(InternalServerError, http.StatusInternalServerError)
	}

	// 获取用户信息
	userInfo, err := ctl.wc.QueryUserInfo(ctx, accessToken.AccessToken, accessToken.OpenID)
	if err != nil {
		log.WithFields(log.Fields{"code": code}).Errorf("failed to get wechat user info: %s", err)
		return webCtx.JSONError(InternalServerError, http.StatusInternalServerError)
	}

	log.WithFields(log.Fields{
		"code":         code,
		"access_token": accessToken,
		"user_info":    userInfo,
	}).Debugf("wechat access token")

	bond, err := ctl.repo.User.WeChatIsBond(ctx, userInfo.UnionID)
	if err != nil {
		log.WithFields(log.Fields{"code": code}).Errorf("failed to check wechat binding: %s", err)
		return webCtx.JSONError(InternalServerError, http.StatusInternalServerError)
	}

	payload := token.Claims{
		"union_id": userInfo.UnionID,
		"nickname": userInfo.NickName,
		"avatar":   userInfo.HeadImgURL,
	}

	return webCtx.JSON(web.M{
		"token": ctl.tk.CreateToken(payload, 5*time.Minute),
		"exist": bond,
	})
}

// SignInWithWechat 使用微信登录
func (ctl *AuthController) SignInWithWechat(ctx context.Context, webCtx web.Context) web.Response {
	accessToken := webCtx.Input("token")
	if accessToken == "" {
		return webCtx.JSONError("token 不能为空", http.StatusBadRequest)
	}

	payload, err := ctl.tk.ParseToken(accessToken)
	if err != nil {
		return webCtx.JSONError("token 无效", http.StatusBadRequest)
	}

	unionID := payload.StringValue("union_id")
	nickname := payload.StringValue("nickname")
	avatar := payload.StringValue("avatar")

	if unionID == "" {
		return webCtx.JSONError("token 无效", http.StatusBadRequest)
	}

	user, eventID, err := ctl.repo.User.SignInWithWeChat(ctx, unionID, nickname, avatar)
	if err != nil {
		log.WithFields(log.Fields{"token": accessToken}).Errorf("failed to sign in with wechat: %s", err)
		return webCtx.JSONError(InternalServerError, http.StatusInternalServerError)
	}

	if eventID > 0 {
		payload := tasks.SignupPayload{
			UserID:    user.Id,
			EventID:   eventID,
			CreatedAt: time.Now(),
		}

		if _, err := ctl.queue.Enqueue(ctx, &payload, asynq.Queue("user")); err != nil {
			log.WithFields(log.Fields{
				"user_id":  user.Id,
				"event_id": eventID,
			}).Errorf("failed to enqueue signup task: %s", err)
		}
	}

	return webCtx.JSON(buildUserLoginRes(user, eventID > 0, ctl.tk))
}

// BindWeChat 绑定微信
func (ctl *AuthController) BindWeChat(ctx context.Context, webCtx web.Context, user *auth.User) web.Response {
	code := strings.TrimSpace(webCtx.Input("code"))
	if code == "" {
		return webCtx.JSONError("code 不能为空", http.StatusBadRequest)
	}

	accessToken, err := ctl.wc.OAuthAccessToken(ctx, code)
	if err != nil {
		log.WithFields(log.Fields{"code": code}).Errorf("failed to get wechat access token: %s", err)
		return webCtx.JSONError(InternalServerError, http.StatusInternalServerError)
	}

	// 获取用户信息
	userInfo, err := ctl.wc.QueryUserInfo(ctx, accessToken.AccessToken, accessToken.OpenID)
	if err != nil {
		log.WithFields(log.Fields{"code": code}).Errorf("failed to get wechat user info: %s", err)
		return webCtx.JSONError(InternalServerError, http.StatusInternalServerError)
	}

	log.WithFields(log.Fields{
		"code":         code,
		"access_token": accessToken,
		"user_info":    userInfo,
	}).Debugf("wechat access token")

	if err := ctl.repo.User.BindWeChat(ctx, user.ID, userInfo.UnionID, userInfo.NickName, userInfo.HeadImgURL); err != nil {
		log.WithFields(log.Fields{
			"user_id":  user.ID,
			"union_id": userInfo.UnionID,
		}).Errorf("failed to bind wechat: %s", err)
		return webCtx.JSONError(err.Error(), http.StatusBadRequest)
	}

	return webCtx.JSON(web.M{})
}

// SignInWithApple 使用 Apple ID 登录
func (ctl *AuthController) SignInWithApple(ctx context.Context, webCtx web.Context) web.Response {
	authorizationCode := strings.TrimSpace(webCtx.Input("authorization_code"))
	if authorizationCode == "" {
		return webCtx.JSONError("authorization_code is required", http.StatusBadRequest)
	}

	givenName := webCtx.Input("given_name")
	familyName := webCtx.Input("family_name")
	inviteCode := strings.TrimSpace(webCtx.Input("invite_code"))

	logFields := log.Fields{
		"email":              webCtx.Input("email"),
		"user_identifier":    webCtx.Input("user_identifier"),
		"given_name":         givenName,
		"family_name":        familyName,
		"authorization_code": authorizationCode,
		"identity_token":     webCtx.Input("identity_token"),
		"is_ios":             webCtx.Input("is_ios"),
	}
	log.WithFields(logFields).Debugf("sign in with apple")

	user, isNewUser, err := appleSignIn(ctx, webCtx.Input("is_ios") == "true", ctl.conf, ctl.repo.User, ctl.queue, authorizationCode, familyName, givenName, inviteCode)
	if err != nil {
		log.WithFields(logFields).Error(err.Error())
		return webCtx.JSONError(InternalServerError, http.StatusInternalServerError)
	}

	// 微信绑定 token
	wechatBindToken := strings.TrimSpace(webCtx.Input("wechat_bind_token"))
	if wechatBindToken != "" {
		if err := ctl.bindWeChatWithToken(ctx, user.Id, wechatBindToken); err != nil {
			log.WithFields(log.Fields{
				"user_id": user.Id,
				"token":   wechatBindToken,
			}).Errorf("failed to bind wechat: %s", err)
		}
	}

	return webCtx.JSON(buildUserLoginRes(user, isNewUser, ctl.tk))
}

func appleSignIn(
	ctx context.Context,
	isIOS bool,
	conf *config.Config,
	userRepo *repo.UserRepo,
	qu *queue.Queue,
	authorizationCode string,
	familyName, givenName string,
	inviteCode string,
) (*model.Users, bool, error) {

	clientID := ternary.If(isIOS, "cc.aicode.flutter.askaide.askaide", "cc.aicode.askaide")
	secret, err := apple.GenerateClientSecret(
		conf.Apple.Secret,
		conf.Apple.TeamID,
		clientID,
		conf.Apple.KeyID,
	)
	if err != nil {
		return nil, false, fmt.Errorf("generate client secret failed: %s", err)
	}

	client := apple.New()
	req := apple.AppValidationTokenRequest{
		ClientID:     clientID,
		ClientSecret: secret,
		Code:         authorizationCode,
	}

	var resp apple.ValidationResponse
	if err := client.VerifyAppToken(ctx, req, &resp); err != nil {
		return nil, false, fmt.Errorf("verify app token failed: %s", err)
	}

	if resp.Error != "" {
		return nil, false, fmt.Errorf("verify app token failed: %s(%s)", resp.Error, resp.ErrorDescription)
	}

	unique, err := apple.GetUniqueID(resp.IDToken)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get unique ID: %s", err)
	}

	claim, err := apple.GetClaims(resp.IDToken)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get claims: %s", err)
	}

	log.With(claim).Debug("apple signin claims")

	email := (*claim)["email"].(string)
	// emailVerified := (*claim)["email_verified"].(bool)
	isPrivateEmail := misc.ClaimBool(claim, "is_private_email")

	user, eventID, err := userRepo.SignInWithApple(ctx, unique, email, isPrivateEmail, familyName, givenName)
	if err != nil {
		return nil, false, fmt.Errorf("failed to sign in with apple: %s", err)
	}

	if eventID > 0 {
		payload := tasks.SignupPayload{
			UserID:     user.Id,
			Email:      email,
			EventID:    eventID,
			InviteCode: inviteCode,
			CreatedAt:  time.Now(),
		}

		if _, err := qu.Enqueue(ctx, &payload, asynq.Queue("user")); err != nil {
			log.WithFields(log.Fields{
				"username": email,
				"event_id": eventID,
			}).Errorf("failed to enqueue signup task: %s", err)
		}
	}

	return user, eventID > 0, nil
}

// buildUserLoginRes 构建用户登录响应
func buildUserLoginRes(user *model.Users, isSignup bool, tk *token.Token) web.M {
	if user.Phone != "" {
		user.Phone = misc.MaskPhoneNumber(user.Phone)
	}

	return web.M{
		"id":          user.Id,
		"name":        user.Realname,
		"email":       user.Email,
		"phone":       user.Phone,
		"is_new_user": isSignup,
		"reward":      coins.BindPhoneGiftCoins,
		"token": tk.CreateToken(token.Claims{
			"id": user.Id,
		}, 6*30*24*time.Hour),
	}
}
