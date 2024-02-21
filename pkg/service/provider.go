package service

import "github.com/mylxsw/glacier/infra"

type Provider struct{}

func (Provider) Register(binder infra.Binder) {
	binder.MustSingleton(NewUserService)
	binder.MustSingleton(func(resolver infra.Resolver) *Service {
		srv := Service{}
		resolver.MustAutoWire(&srv)
		return &srv
	})
}

type Service struct {
	User *UserService `autowire:"@"`
}
