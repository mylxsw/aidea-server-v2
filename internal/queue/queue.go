package queue

import (
	"context"
	"encoding/json"
	"github.com/hashicorp/go-uuid"
	"github.com/hibiken/asynq"
	"github.com/mylxsw/aidea-chat-server/pkg/repo"
	"github.com/mylxsw/go-utils/must"
	"time"
)

// CompletionResult 任务完成后的结果
type CompletionResult struct {
	Resources   []string  `json:"resources"`
	OriginImage string    `json:"origin_image,omitempty"`
	ValidBefore time.Time `json:"valid_before,omitempty"`
	Width       int64     `json:"width,omitempty"`
	Height      int64     `json:"height,omitempty"`
}

// ErrorResult 任务失败后的结果
type ErrorResult struct {
	Errors []string `json:"errors"`
}

type EmptyResult struct{}

// TaskHandler 任务处理器
type TaskHandler func(context.Context, *asynq.Task) error

// Payload 任务载荷接口
type Payload interface {
	GetTitle() string
	GetID() string
	SetID(id string)
	GetType() string
}

// Queue 任务队列
type Queue struct {
	client    *asynq.Client
	queueRepo *repo.QueueRepo
}

// NewQueue 创建一个任务队列
func NewQueue(client *asynq.Client, queueRepo *repo.QueueRepo) *Queue {
	return &Queue{client: client, queueRepo: queueRepo}
}

// Enqueue 将任务加入队列
func (q *Queue) Enqueue(ctx context.Context, payload Payload, opts ...asynq.Option) (string, error) {
	payload.SetID(must.Must(uuid.GenerateUUID()))

	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	task := asynq.NewTask(payload.GetType(), data)
	info, err := q.client.Enqueue(task, opts...)
	if err != nil {
		return "", err
	}

	return payload.GetID(), q.queueRepo.Add(
		ctx,
		payload.GetID(),
		task.Type(),
		info.Queue,
		payload.GetTitle(),
		task.Payload(),
	)
}
