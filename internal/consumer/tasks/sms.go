package tasks

import (
	"context"
	"encoding/json"
	"github.com/hibiken/asynq"
	"github.com/mylxsw/aidea-chat-server/internal/queue"
	"github.com/mylxsw/aidea-chat-server/pkg/repo"
	"github.com/mylxsw/asteria/log"
	"time"
)

const TypeSMSVerifyCode = "sms:verify_code"

type SMSVerifyCodePayload struct {
	ID        string    `json:"id,omitempty"`
	Receiver  string    `json:"receiver"`
	Code      string    `json:"code"`
	CreatedAt time.Time `json:"created_at"`
}

func (payload *SMSVerifyCodePayload) GetType() string {
	return TypeSMSVerifyCode
}

func (payload *SMSVerifyCodePayload) GetTitle() string {
	return "短信验证码"
}

func (payload *SMSVerifyCodePayload) SetID(id string) {
	payload.ID = id
}

func (payload *SMSVerifyCodePayload) GetID() string {
	return payload.ID
}

func RegisterSMSTask(mux *asynq.ServeMux, rp *repo.Repository) {
	mux.HandleFunc(TypeSMSVerifyCode, func(ctx context.Context, task *asynq.Task) (err error) {
		var payload SMSVerifyCodePayload
		if err := json.Unmarshal(task.Payload(), &payload); err != nil {
			return err
		}

		// 如果任务是 5 分钟前创建的，不再处理
		if payload.CreatedAt.Add(5 * time.Minute).Before(time.Now()) {
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
			}
		}()

		// TODO 发送短信验证码
		log.F(log.M{"code": payload.Code}).Infof("send sms code to %s", payload.Receiver)

		return rp.Queue.Update(
			context.TODO(),
			payload.GetID(),
			repo.QueueTaskStatusSuccess,
			queue.EmptyResult{},
		)
	})
}
