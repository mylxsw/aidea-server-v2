package repo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/mylxsw/aidea-chat-server/config"
	"github.com/mylxsw/aidea-chat-server/pkg/misc"
	"github.com/mylxsw/aidea-chat-server/pkg/repo/model"
	"github.com/mylxsw/eloquent/query"
	"gopkg.in/guregu/null.v3"
	"time"
)

type QueueTaskStatus string

const (
	QueueTaskStatusPending QueueTaskStatus = "pending"
	QueueTaskStatusRunning QueueTaskStatus = "running"
	QueueTaskStatusSuccess QueueTaskStatus = "success"
	QueueTaskStatusFailed  QueueTaskStatus = "failed"
)

type QueueRepo struct {
	db   *sql.DB
	conf *config.Config
}

func NewQueueRepo(db *sql.DB, conf *config.Config) *QueueRepo {
	return &QueueRepo{db: db, conf: conf}
}

func (repo *QueueRepo) Add(ctx context.Context, taskID, taskType, queueName string, title string, payload []byte) error {
	_, err := model.NewQueueTasksModel(repo.db).Create(ctx, query.KV{
		model.FieldQueueTasksTitle:     misc.SubString(title, 70),
		model.FieldQueueTasksTaskId:    taskID,
		model.FieldQueueTasksTaskType:  taskType,
		model.FieldQueueTasksQueueName: queueName,
		model.FieldQueueTasksStatus:    QueueTaskStatusPending,
		model.FieldQueueTasksPayload:   null.StringFrom(string(payload)),
	})

	return err
}

func (repo *QueueRepo) Update(ctx context.Context, taskID string, status QueueTaskStatus, result any) error {
	task, err := model.NewQueueTasksModel(repo.db).First(ctx, query.Builder().Where(model.FieldQueueTasksTaskId, taskID))
	if err != nil {
		return err
	}

	task.Status = null.StringFrom(string(status))

	if result != nil {
		data, err := json.Marshal(result)
		if err != nil {
			return err
		}
		task.Result = null.StringFrom(string(data))
	}

	return task.Save(ctx, model.FieldQueueTasksStatus, model.FieldQueueTasksResult)
}

func (repo *QueueRepo) Task(ctx context.Context, taskID string) (*model.QueueTasks, error) {
	task, err := model.NewQueueTasksModel(repo.db).First(ctx, query.Builder().Where(model.FieldQueueTasksTaskId, taskID))
	if err != nil {
		if errors.Is(err, query.ErrNoResult) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	res := task.ToQueueTasks()
	return &res, nil
}

func (repo *QueueRepo) Remove(ctx context.Context, taskID string) error {
	_, err := model.NewQueueTasksModel(repo.db).Delete(ctx, query.Builder().Where(model.FieldQueueTasksTaskId, taskID))
	return err
}

func (repo *QueueRepo) RemoveQueueTasks(ctx context.Context, before time.Time) error {
	q := query.Builder().
		Where(model.FieldQueueTasksCreatedAt, "<", before).
		Where(model.FieldQueueTasksStatus, QueueTaskStatusSuccess)
	_, err := model.NewQueueTasksModel(repo.db).Delete(ctx, q)
	return err
}
