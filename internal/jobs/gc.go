package jobs

import (
	"context"
	"github.com/mylxsw/aidea-chat-server/pkg/repo"
	"github.com/mylxsw/asteria/log"
	"time"
)

func ClearExpiredTaskJob(ctx context.Context, queueRepo *repo.QueueRepo) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	// 清理过期的 QueueTasks
	if err := queueRepo.RemoveQueueTasks(ctx, time.Now().AddDate(0, 0, -3)); err != nil {
		log.Errorf("清理过期的 QueueTasks 失败: %v", err)
	}

	return nil
}

func ClearExpiredCacheJob(ctx context.Context, cacheRepo *repo.CacheRepo) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	if err := cacheRepo.GC(ctx); err != nil {
		log.Errorf("清理过期的缓存失败: %v", err)
	}

	return nil
}
