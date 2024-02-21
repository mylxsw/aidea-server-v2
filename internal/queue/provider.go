package queue

import (
	"github.com/hibiken/asynq"
	"github.com/mylxsw/aidea-chat-server/config"
	"github.com/mylxsw/glacier/infra"
)

type Provider struct{}

func (Provider) Register(binder infra.Binder) {
	binder.MustSingleton(func(conf *config.Config) *asynq.Client {
		return asynq.NewClient(asynq.RedisClientOpt{
			Addr:     conf.RedisAddr(),
			Password: conf.RedisPassword,
		})
	})

	binder.MustSingleton(NewQueue)
}

func (Provider) Boot(app infra.Resolver) {

}
