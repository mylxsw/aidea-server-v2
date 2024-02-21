package proxy

import (
	"github.com/mylxsw/aidea-chat-server/config"
	"github.com/mylxsw/asteria/log"
	"github.com/mylxsw/glacier/infra"
	"net/http"
	"net/url"
)

type Provider struct{}

type Proxy struct {
	http func(*http.Request) (*url.URL, error)
}

func (pp *Proxy) BuildTransport() *http.Transport {
	return &http.Transport{Proxy: pp.http}
}

func (Provider) Register(binder infra.Binder) {
	binder.MustSingleton(func(conf *config.Config) (*Proxy, error) {
		pp := &Proxy{}

		if conf.ProxyURL == "" {
			pp.http = http.ProxyFromEnvironment
		} else {
			p, err := url.Parse(conf.ProxyURL)
			if err != nil {
				log.Errorf("invalid proxy url: %s", conf.ProxyURL)
				return nil, err
			}

			pp.http = http.ProxyURL(p)
		}

		return pp, nil
	})
}
