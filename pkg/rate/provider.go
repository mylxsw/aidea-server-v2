package rate

import "github.com/mylxsw/glacier/infra"

type Provider struct{}

func (Provider) Register(binder infra.Binder) {
	binder.MustSingleton(NewLimiter)
	binder.MustSingleton(New)
}
