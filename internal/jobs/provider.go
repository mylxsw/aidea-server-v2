package jobs

import (
	"github.com/mylxsw/aidea-chat-server/config"
	"time"

	"github.com/mylxsw/asteria/log"
	"github.com/mylxsw/glacier/infra"
	"github.com/mylxsw/glacier/scheduler"
	"github.com/redis/go-redis/v9"
	cronV3 "github.com/robfig/cron/v3"
)

type Provider struct{}

func (p Provider) Aggregates() []infra.Provider {
	return []infra.Provider{
		scheduler.Provider(
			p.Jobs,
			scheduler.SetLockManagerOption(func(resolver infra.Resolver) scheduler.LockManagerBuilder {
				redisClient := resolver.MustGet(&redis.Client{}).(*redis.Client)
				return func(name string) scheduler.LockManager {
					return New(redisClient, name, 1*time.Minute)
				}
			}),
		),
	}
}

func (Provider) Register(binder infra.Binder) {
	binder.MustSingleton(func() *cronV3.Cron {
		log.Debugf("initialize the scheduled task manager, location=%s", time.Local.String())
		return cronV3.New(
			cronV3.WithSeconds(),
			cronV3.WithLogger(cronLogger{}),
			cronV3.WithLocation(time.Local),
		)
	})
}

func (Provider) Jobs(resolver infra.Resolver, creator scheduler.JobCreator) {

	resolver.MustResolve(func(conf *config.Config) {
		if !conf.EnableScheduler {
			return
		}

		jobs(creator)
	})

}
