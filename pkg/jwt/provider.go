package jwt

import (
	"github.com/mylxsw/aidea-chat-server/config"
	"github.com/mylxsw/glacier/infra"
)

type Provider struct{}

func (Provider) Register(binder infra.Binder) {
	binder.MustSingleton(func(conf *config.Config) *Token {
		return New(conf.SessionSecret)
	})
}
