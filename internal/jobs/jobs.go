package jobs

import (
	"github.com/mylxsw/aidea-chat-server/pkg/misc"
	"github.com/mylxsw/glacier/scheduler"
)

func jobs(creator scheduler.JobCreator) {
	// 清理过期任务
	misc.NoError(creator.Add(
		"clear-expired-task",
		"0 0 0 * * *",
		scheduler.WithoutOverlap(ClearExpiredTaskJob),
	))

	// 清理过期缓存
	misc.NoError(creator.Add(
		"clear-expired-cache",
		"0 0 0 * * *",
		scheduler.WithoutOverlap(ClearExpiredCacheJob),
	))
}
