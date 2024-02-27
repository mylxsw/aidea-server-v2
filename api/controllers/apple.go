package controllers

import (
	"context"
	"fmt"
	"github.com/mylxsw/aidea-chat-server/config"
	"github.com/mylxsw/aidea-chat-server/pkg/jwt"
	"github.com/mylxsw/aidea-chat-server/pkg/repo"
	"github.com/mylxsw/asteria/log"
	"github.com/mylxsw/glacier/infra"
	"github.com/mylxsw/glacier/web"
	"net/http"
	"net/url"
)

type AppleAuthController struct {
	conf *config.Config `autowire:"@"`
}

func NewAppleAuthController(resolver infra.Resolver) web.Controller {
	ctl := AppleAuthController{}
	resolver.MustAutoWire(&ctl)
	return &ctl
}

func (ctl *AppleAuthController) Register(router web.Router) {
	router.Group("/callback/auth", func(router web.Router) {
		router.Any("/sign_in_with_apple", ctl.signInWithApple)
	})
}

func (ctl *AppleAuthController) signInWithApple(ctx context.Context, webCtx web.Context, userRepo *repo.UserRepo, tk *jwt.Token) web.Response {
	log.WithFields(log.Fields{
		"code":     webCtx.Input("code"),
		"id_token": webCtx.Input("id_token"),
	}).Debugf("apple auth callback")

	params := url.Values{}
	params.Add("code", webCtx.Input("code"))
	params.Add("id_token", webCtx.Input("id_token"))
	return webCtx.Redirect(
		fmt.Sprintf("intent://callback?%s#Intent;package=cc.aicode.flutter.askaide.askaide;scheme=signinwithapple;end", params.Encode()),
		http.StatusTemporaryRedirect,
	)
}
