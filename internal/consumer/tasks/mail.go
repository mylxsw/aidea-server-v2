package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/hibiken/asynq"
	"github.com/mylxsw/aidea-chat-server/internal/queue"
	"github.com/mylxsw/aidea-chat-server/pkg/mail"
	"github.com/mylxsw/aidea-chat-server/pkg/repo"
	"github.com/mylxsw/asteria/log"
	"time"
)

const TypeMailSend = "mail:send"

type MailPayload struct {
	ID        string    `json:"id,omitempty"`
	To        []string  `json:"to"`
	Subject   string    `json:"subject"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

func (payload *MailPayload) GetType() string {
	return TypeMailSend
}

func (payload *MailPayload) GetTitle() string {
	return payload.Subject
}

func (payload *MailPayload) SetID(id string) { payload.ID = id }

func (payload *MailPayload) GetID() string {
	return payload.ID
}

func RegisterMailSendTask(mux *asynq.ServeMux, mailer *mail.Sender, rp *repo.Repository) {
	mux.HandleFunc(TypeMailSend, func(ctx context.Context, task *asynq.Task) (err error) {
		var payload MailPayload
		if err := json.Unmarshal(task.Payload(), &payload); err != nil {
			return err
		}

		// 如果任务是 15 分钟前创建的，不再处理
		if payload.CreatedAt.Add(15 * time.Minute).Before(time.Now()) {
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

		if err := mailer.Send(payload.To, fmt.Sprintf("【AIdea】%s", payload.Subject), payload.Body); err != nil {
			log.With(payload).Errorf("send mail failed: %v", err)
			return err
		}

		return rp.Queue.Update(
			context.TODO(),
			payload.GetID(),
			repo.QueueTaskStatusSuccess,
			queue.EmptyResult{},
		)
	})
}
