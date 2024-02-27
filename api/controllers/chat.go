package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-redis/redis_rate/v10"
	"github.com/mylxsw/aidea-chat-server/api/auth"
	"github.com/mylxsw/aidea-chat-server/config"
	"github.com/mylxsw/aidea-chat-server/pkg/chat"
	"github.com/mylxsw/aidea-chat-server/pkg/misc"
	"github.com/mylxsw/aidea-chat-server/pkg/rate"
	"github.com/mylxsw/asteria/log"
	"github.com/mylxsw/glacier/infra"
	"github.com/mylxsw/glacier/web"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ChatController is the controller for chat
type ChatController struct {
	conf    *config.Config `autowire:"@"`
	limiter *rate.Limiter  `autowire:"@"`
	chatter *chat.Chatter  `autowire:"@"`
}

func NewChatController(resolver infra.Resolver) web.Controller {
	ctl := &ChatController{}
	resolver.MustAutoWire(ctl)
	return ctl
}

func (ctl *ChatController) Register(router web.Router) {
	router.Group("/chat", func(router web.Router) {
		router.Post("/stream", ctl.ChatStream)
	})
}

// ChatRequest chat request
type ChatRequest struct {
	chat.Request
}

// Init initialize chat request
func (req ChatRequest) Init() ChatRequest {
	return req
}

// ChatStream request handler
func (ctl *ChatController) ChatStream(
	ctx context.Context,
	webCtx web.Context,
	user *auth.UserOptional,
	client *auth.ClientInfo,
	w http.ResponseWriter,
) {
	if user.User == nil && ctl.conf.EnableAnonymousChat {
		user.User = &auth.User{}
	}

	if user.User == nil {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error": "the user is not logged in, please log in first and try again"}`))
		return
	}

	// rate control to avoid overuse by a single user
	if err := ctl.rateLimit(ctx, client, user.User); err != nil {
		if errors.Is(err, rate.ErrDailyFreeLimitExceeded) {
			w.WriteHeader(http.StatusUnauthorized)
		} else {
			w.WriteHeader(http.StatusTooManyRequests)
		}
		_, _ = w.Write([]byte(fmt.Sprintf(`{"error": %s}`, strconv.Quote(err.Error()))))
		return
	}

	sw, req, err := misc.NewStreamWriter[ChatRequest](
		webCtx.Input("ws") == "true", ctl.conf.EnableCORS, webCtx.Request().Raw(), w,
	)
	if err != nil {
		log.F(log.M{"user": user.User.ID, "client": client}).Errorf("create stream writer failed: %s", err)
		return
	}
	defer sw.Close()

	startTime := time.Now()

	// save chat question
	questionID := ctl.saveChatQuestion(ctx, req, user.User)

	var replyText string
	var usage *chat.Usage

	defer func() {
		log.F(log.M{
			"user_id":     user.User.ID,
			"client":      client,
			"req":         req,
			"question_id": questionID,
			"reply":       replyText,
			"usage":       usage,
			"elapse":      time.Since(startTime).Seconds(),
		}).
			Infof("chat request finished")

		// chat result processing
		ctl.handleChatResult(ctx, sw, req, user.User, questionID, replyText, usage, err)
	}()

	// handle chat request
	replyText, usage, err = ctl.handleChat(ctx, sw, req, 0)
	if errors.Is(err, ErrChatResponseHasSent) {
		return
	}

	// Try again in the following two situations
	// 1. ChatStream response is empty
	// 2. The waiting time between two responses is too long, forced interruption, and the response is empty.
	if errors.Is(err, ErrChatResponseEmpty) || (errors.Is(err, ErrChatResponseGapTimeout) && replyText == "") {
		// If the user waits for more than 60 seconds, there will be no retry to prevent the user from waiting too long.
		if startTime.Add(60 * time.Second).After(time.Now()) {
			log.F(log.M{"req": req, "user_id": user.User.ID}).Warningf("chat response is empty, try requesting again")

			replyText, usage, err = ctl.handleChat(ctx, sw, req, 1)
			if errors.Is(err, ErrChatResponseHasSent) {
				return
			}
		}
	}

}

// rateLimit rate limit control
func (ctl *ChatController) rateLimit(ctx context.Context, client *auth.ClientInfo, user *auth.User) error {
	if err := ctl.limiter.Allow(ctx, fmt.Sprintf("chat-limit:u:%d:minute", user.ID), redis_rate.PerMinute(10)); err != nil {
		if errors.Is(err, rate.ErrRateLimitExceeded) {
			return rate.ErrRateLimitExceeded
		}

		log.F(log.M{"user_id": user.ID}).Errorf("frequency of chat requests is too high: %s", err)
	}

	return nil
}

var (
	ErrChatResponseEmpty      = errors.New("chat response is empty")
	ErrChatResponseHasSent    = errors.New("chat response has been sent")
	ErrChatResponseGapTimeout = errors.New("waiting time between two responses is too long, forced interruption")
)

// ChatResponse chat response
type ChatResponse struct {
	chat.StreamResponse
}

// handleChat handle chat request
func (ctl *ChatController) handleChat(ctx context.Context, sw *misc.StreamWriter, req *ChatRequest, retryTimes int) (string, *chat.Usage, error) {
	ctx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()

	if retryTimes > 0 {
		ctx = chat.NewContext(ctx, &chat.Control{PreferBackup: true})
	}

	stream, err := ctl.chatter.ChatStream(ctx, req.Request)
	if err != nil {
		if errors.Is(err, chat.ErrContentFilter) {
			ctl.writeViolateContextPolicyError(sw, err.Error())
			return "", nil, ErrChatResponseHasSent
		}

		log.F(log.M{"req": req, "retry_times": retryTimes}).Errorf("chat stream failed: %s", err)
		misc.NoError(sw.WriteErrorStream(err, http.StatusInternalServerError))
		return "", nil, ErrChatResponseHasSent
	}

	replyText, usage, err := ctl.handleChatRequest(ctx, sw, stream)
	if err != nil {
		return replyText, usage, err
	}

	replyText = strings.TrimSpace(replyText)
	if replyText == "" {
		return replyText, usage, ErrChatResponseEmpty
	}

	return replyText, usage, nil
}

// handleChatRequest handle chat request
func (ctl *ChatController) handleChatRequest(ctx context.Context, sw *misc.StreamWriter, stream <-chan chat.StreamResponse) (replyText string, usage *chat.Usage, err error) {
	timer := time.NewTimer(60 * time.Second)
	defer timer.Stop()

	id := 0
	for {
		if id > 0 {
			timer.Reset(30 * time.Second)
		}

		select {
		case <-timer.C:
			return replyText, usage, ErrChatResponseGapTimeout
		case <-ctx.Done():
			return replyText, usage, nil
		case res, ok := <-stream:
			if !ok {
				return replyText, usage, nil
			}

			id++

			if res.ErrorCode != "" {
				if res.ErrorCode != "" {
					res.ErrorMessage = fmt.Sprintf("\n\n---\nSorry, we encountered some errors, here are the error details:\n%s\n", res.ErrorMessage)
				} else {
					return replyText, usage, nil
				}
			} else {
				replyText += res.DeltaText()
				if res.Usage != nil {
					usage = res.Usage
				}
			}

			resp := ChatResponse{StreamResponse: res}

			if err := sw.WriteStream(resp); err != nil {
				return replyText, usage, nil
			}
		}
	}
}

type UsageSummary struct {
	QuestionID int64       `json:"question_id,omitempty"`
	AnswerID   int64       `json:"answer_id,omitempty"`
	Quota      int64       `json:"quota,omitempty"`
	Error      string      `json:"error,omitempty"`
	Usage      *chat.Usage `json:"usage,omitempty"`
}

func (usage UsageSummary) JSON() string {
	res, _ := json.Marshal(usage)
	return string(res)
}

// saveChatQuestion save chat question and return question id
func (ctl *ChatController) saveChatQuestion(ctx context.Context, req *ChatRequest, user *auth.User) int64 {
	// TODO 保存聊天问题
	return 0
}

func (ctl *ChatController) handleChatResult(ctx context.Context, sw *misc.StreamWriter, req *ChatRequest, user *auth.User, questionID int64, replyText string, usage *chat.Usage, err error) {
	// TODO 保存聊天结果
	// 如果 error 不为空，则需要更新 question 的状态为 失败
	answerID := int64(1)

	// 更新智慧果消耗
	quotaConsumed := int64(0)

	// 告知客户端实际消耗情况
	summary := UsageSummary{
		QuestionID: questionID,
		AnswerID:   answerID,
		Usage:      usage,
		Quota:      quotaConsumed,
	}

	if err != nil {
		summary.Error = err.Error()
	}

	_ = sw.WriteStream(chat.NewSystemStreamResponse("final", summary.JSON(), "").JSON())
}

const violateContentPolicyMessage = "抱歉，您的请求因包含违规内容被系统拦截，如果您对此有任何疑问或想进一步了解详情，欢迎通过以下渠道与我们联系：\n\n服务邮箱：support@aicode.cc\n\n微博：@mylxsw\n\n客服微信：x-prometheus\n\n\n---\n\n> 本次请求不扣除智慧果。"

func (ctl *ChatController) writeViolateContextPolicyError(sw *misc.StreamWriter, detail string) {
	reason := violateContentPolicyMessage
	if detail != "" {
		reason += fmt.Sprintf("\n> \n> 原因：%s", detail)
	}

	_ = sw.WriteStream(chat.NewStreamResponse("content_filter", reason, "content_filter").JSON())
}
