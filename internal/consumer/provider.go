package consumer

import (
	"context"
	"github.com/mylxsw/aidea-chat-server/config"
	"github.com/mylxsw/aidea-chat-server/internal/consumer/tasks"
	"github.com/mylxsw/asteria/log"
	"github.com/mylxsw/glacier/infra"
	"time"

	"github.com/hibiken/asynq"
)

type Provider struct{}

func (Provider) Register(binder infra.Binder) {
	binder.MustSingleton(func(conf *config.Config) *asynq.Server {
		return asynq.NewServer(
			asynq.RedisClientOpt{
				Addr:     conf.RedisAddr(),
				Password: conf.RedisPassword,
			},
			asynq.Config{
				Concurrency: conf.QueueWorkers,
				Queues: map[string]int{
					"mail":    conf.QueueWorkers / 5 * 1,
					"user":    conf.QueueWorkers / 5 * 1,
					"default": conf.QueueWorkers - conf.QueueWorkers/5*2,
					//"text":  conf.QueueWorkers / 3 * 2,
					//"image": conf.QueueWorkers - conf.QueueWorkers/3*2,
				},
				Logger: Logger{},
			},
		)
	})

	binder.MustSingleton(func(server *asynq.Server) *asynq.ServeMux {
		mux := asynq.NewServeMux()
		mux.Use(loggingMiddleware)
		return mux
	})
}

func loggingMiddleware(h asynq.Handler) asynq.Handler {
	return asynq.HandlerFunc(func(ctx context.Context, t *asynq.Task) error {
		start := time.Now()
		log.Debugf("Start processing %q", t.Type())
		err := h.ProcessTask(ctx, t)
		if err != nil {
			log.Warningf("task process failed: %q, %v", t.Type(), err)
			// 失败后不再进行重试
			return asynq.SkipRetry
		}

		log.Debugf("finished processing %q: elapsed time = %v", t.Type(), time.Since(start))
		return nil
	})
}

func (p Provider) Boot(resolver infra.Resolver) {
	log.Debugf("register all queue handlers")

	resolver.MustResolve(tasks.RegisterMailSendTask)
	resolver.MustResolve(tasks.RegisterBindPhoneTask)
	resolver.MustResolve(tasks.RegisterSignupTask)
	resolver.MustResolve(tasks.RegisterSMSTask)
}

func (Provider) ShouldLoad(conf *config.Config) bool {
	return conf.QueueWorkers > 0
}

func (Provider) Daemon(ctx context.Context, resolver infra.Resolver) {
	resolver.MustResolve(func(conf *config.Config, server *asynq.Server, mux *asynq.ServeMux) error {
		log.Debugf("start queue consumer")
		return server.Run(mux)
	})
}
