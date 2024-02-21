package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/mylxsw/aidea-chat-server/config"
	"github.com/mylxsw/aidea-chat-server/internal/queue"
	"github.com/mylxsw/aidea-chat-server/pkg/repo"
	"github.com/mylxsw/asteria/log"
	"github.com/mylxsw/glacier/infra"
	"github.com/mylxsw/glacier/web"
	"net/http"
	"time"
)

type TaskController struct {
	conf *config.Config   `autowire:"@"`
	repo *repo.Repository `autowire:"@"`
}

func NewTaskController(resolver infra.Resolver) web.Controller {
	ctl := TaskController{}
	resolver.MustAutoWire(&ctl)
	return &ctl
}

func (ctl *TaskController) Register(router web.Router) {
	router.Group("/tasks", func(router web.Router) {
		router.Get("/{task_id}/status", ctl.TaskStatus)
	})
}

// TaskStatus 任务状态查询
func (ctl *TaskController) TaskStatus(ctx context.Context, webCtx web.Context) web.Response {
	taskID := webCtx.PathVar("task_id")
	task, err := ctl.repo.Queue.Task(ctx, taskID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return webCtx.JSONError(NotFoundError, http.StatusNotFound)
		}
		return webCtx.JSONError(InternalServerError, http.StatusInternalServerError)
	}

	if repo.QueueTaskStatus(task.Status) == repo.QueueTaskStatusSuccess {
		var taskResult queue.CompletionResult
		if err := json.Unmarshal([]byte(task.Result), &taskResult); err != nil {
			log.With(task).Errorf("unmarshal task result failed: %v", err)
			return webCtx.JSONError(InternalServerError, http.StatusInternalServerError)
		}
		res := web.M{
			"status":       task.Status,
			"origin_image": taskResult.OriginImage,
			"resources":    taskResult.Resources,
			"valid_before": taskResult.ValidBefore.Format(time.RFC3339),
		}

		if taskResult.Width > 0 {
			res["width"] = taskResult.Width
		}

		if taskResult.Height > 0 {
			res["height"] = taskResult.Height
		}

		return webCtx.JSON(res)
	}

	if repo.QueueTaskStatus(task.Status) == repo.QueueTaskStatusFailed {
		var errResult queue.ErrorResult
		if err := json.Unmarshal([]byte(task.Result), &errResult); err != nil {
			log.With(task).Errorf("unmarshal task result failed: %v", err)
			return webCtx.JSONError(InternalServerError, http.StatusInternalServerError)
		}

		return webCtx.JSON(web.M{
			"status": task.Status,
			"errors": errResult.Errors,
		})
	}

	return webCtx.JSON(web.M{
		"status": task.Status,
	})
}
