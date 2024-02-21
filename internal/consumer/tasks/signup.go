package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/hibiken/asynq"
	"github.com/mylxsw/aidea-chat-server/internal/coins"
	"github.com/mylxsw/aidea-chat-server/internal/queue"
	"github.com/mylxsw/aidea-chat-server/pkg/repo"
	"github.com/mylxsw/asteria/log"
	"time"
)

const TypeSignup = "user:signup"

type SignupPayload struct {
	ID         string    `json:"id,omitempty"`
	UserID     int64     `json:"user_id"`
	Email      string    `json:"email"`
	Phone      string    `json:"phone"`
	InviteCode string    `json:"invite_code"`
	EventID    int64     `json:"event_id"`
	CreatedAt  time.Time `json:"created_at"`

	WeChatUnionID string `json:"wechat_union_id"`
}

func (payload *SignupPayload) GetType() string {
	return TypeSignup
}

func (payload *SignupPayload) GetTitle() string {
	return "用户注册"
}

func (payload *SignupPayload) SetID(id string) {
	payload.ID = id
}

func (payload *SignupPayload) GetID() string {
	return payload.ID
}

func RegisterSignupTask(mux *asynq.ServeMux, rp *repo.Repository) {
	mux.HandleFunc(TypeSignup, func(ctx context.Context, task *asynq.Task) (err error) {
		var payload SignupPayload
		if err := json.Unmarshal(task.Payload(), &payload); err != nil {
			return err
		}

		// 如果任务是 60 分钟前创建的，不再处理
		if payload.CreatedAt.Add(60 * time.Minute).Before(time.Now()) {
			return nil
		}

		defer func() {
			if err2 := recover(); err2 != nil {
				log.With(task).Errorf("panic: %v", err2)
				err = err2.(error)
			}

			if err != nil {
				if err := rp.Queue.Update(
					context.TODO(),
					payload.GetID(),
					repo.QueueTaskStatusFailed,
					queue.ErrorResult{
						Errors: []string{err.Error()},
					},
				); err != nil {
					log.With(task).Errorf("update queue status failed: %s", err)
				}

				if err := rp.Event.UpdateEvent(ctx, payload.EventID, repo.EventStatusFailed); err != nil {
					log.WithFields(log.Fields{"event_id": payload.EventID}).Errorf("update event status failed: %s", err)
				}
			}
		}()

		// 查询事件记录
		event, err := rp.Event.GetEvent(ctx, payload.EventID)
		if err != nil {
			if errors.Is(err, repo.ErrNotFound) {
				log.WithFields(log.Fields{"event_id": payload.EventID}).Errorf("event not found")
				return nil
			}

			log.With(payload).Errorf("get event failed: %s", err)
			return err
		}

		if event.Status != repo.EventStatusWaiting {
			log.WithFields(log.Fields{"event_id": payload.EventID}).Warningf("event status is not waiting")
			return nil
		}

		if event.EventType != repo.EventTypeUserCreated {
			log.With(payload).Errorf("event type is not user_created")
			return nil
		}

		var eventPayload repo.UserCreatedEvent
		if err := json.Unmarshal([]byte(event.Payload), &eventPayload); err != nil {
			log.With(payload).Errorf("unmarshal event payload failed: %s", err)
			return err
		}

		// 为用户分配默认配额
		// 1. 如果是邮箱注册，不赠送智慧果，只有在用户绑定手机后才赠送
		// 2. 如果是手机注册，直接赠送智慧果
		if eventPayload.From == repo.UserCreatedEventSourceEmail || eventPayload.From == repo.UserCreatedEventSourceWechat {
			if coins.SignupGiftCoins > 0 {
				if _, err := rp.Quota.AddUserQuota(ctx, eventPayload.UserID, int64(coins.SignupGiftCoins), time.Now().AddDate(0, 1, 0), "新用户注册赠送", ""); err != nil {
					log.WithFields(log.Fields{"user_id": eventPayload.UserID}).Errorf("create user quota failed: %s", err)
				}
			}
		} else if eventPayload.From == repo.UserCreatedEventSourcePhone {
			if _, err := rp.Quota.AddUserQuota(ctx, eventPayload.UserID, int64(coins.BindPhoneGiftCoins), time.Now().AddDate(0, 1, 0), "新用户注册赠送", ""); err != nil {
				log.WithFields(log.Fields{"user_id": eventPayload.UserID}).Errorf("create user quota failed: %s", err)
			}
		}

		// 为用户生成自己的邀请码
		if err := rp.User.GenerateInviteCode(ctx, payload.UserID); err != nil {
			log.WithFields(log.Fields{"user_id": payload.UserID}).Errorf("生成邀请码失败: %s", err)
		}

		// 更新用户的邀请信息
		if payload.InviteCode != "" {
			inviteByUser, err := rp.User.GetUserByInviteCode(ctx, payload.InviteCode)
			if err != nil {
				if !errors.Is(err, repo.ErrNotFound) {
					log.With(payload).Errorf("通过邀请码查询用户失败: %s", err)
				}
			} else {
				if err := rp.User.UpdateUserInviteBy(ctx, eventPayload.UserID, inviteByUser.Id); err != nil {
					log.WithFields(log.Fields{"user_id": eventPayload.UserID, "invited_by": inviteByUser.Id}).Errorf("更新用户邀请信息失败: %s", err)
				} else {
					// 为邀请人和被邀请人分配智慧果
					inviteGiftHandler(ctx, rp, eventPayload.UserID, inviteByUser.Id)
				}
			}
		}

		// 为用户创建默认的数字人
		//createInitialRooms(ctx, roomRepo, eventPayload.UserID)

		// 更新事件状态
		if err := rp.Event.UpdateEvent(ctx, payload.EventID, repo.EventStatusSucceed); err != nil {
			log.WithFields(log.Fields{"event_id": payload.EventID}).Errorf("update event status failed: %s", err)
		}

		// 发送钉钉通知
		//go func() {
		//	content := fmt.Sprintf(
		//		`有新用户注册啦，账号 %s（ID：%d） 快去看看吧`,
		//		ternary.If(payload.Phone != "", payload.Phone, payload.Email),
		//		payload.UserID,
		//	)
		//	if err := ding.Send(dingding.NewMarkdownMessage("新用户注册啦", content, []string{})); err != nil {
		//		log.WithFields(log.Fields{"user_id": eventPayload.UserID}).Errorf("send dingding message failed: %s", err)
		//	}
		//}()

		return rp.Queue.Update(
			context.TODO(),
			payload.GetID(),
			repo.QueueTaskStatusSuccess,
			queue.EmptyResult{},
		)
	})
}
