package chat

import (
	"github.com/mylxsw/aidea-chat-server/config"
	"github.com/mylxsw/aidea-chat-server/pkg/proxy"
	"github.com/mylxsw/glacier/infra"
)

type Provider struct{}

func (Provider) Register(binder infra.Binder) {
	binder.MustSingleton(NewChatter)
	binder.MustSingleton(func(conf *config.Config, pp *proxy.Proxy) *OpenAIClient {
		return NewOpenAIClient(conf.OpenAI, pp)
	})
}
