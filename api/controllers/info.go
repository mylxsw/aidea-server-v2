package controllers

import (
	"context"
	"fmt"
	"github.com/mylxsw/aidea-chat-server/api/auth"
	"github.com/mylxsw/aidea-chat-server/config"
	"github.com/mylxsw/aidea-chat-server/pkg/misc"
	"github.com/mylxsw/aidea-chat-server/pkg/service"
	"github.com/mylxsw/glacier/infra"
	"github.com/mylxsw/glacier/web"
	"github.com/redis/go-redis/v9"
	"net/http"
)

type InfoController struct {
	conf *config.Config   `autowire:"@"`
	srv  *service.Service `autowire:"@"`
	rds  *redis.Client    `autowire:"@"`
}

func NewInfoController(resolver infra.Resolver) web.Controller {
	ctl := InfoController{}
	resolver.MustAutoWire(&ctl)
	return &ctl
}

func (ctl *InfoController) Register(router web.Router) {
	router.Group("/info", func(router web.Router) {
		router.Get("/capabilities", ctl.Capabilities)
		router.Get("/version", ctl.Version)
		router.Any("/version-check", ctl.VersionCheck)
	})

	router.Any("/r/{key}", ctl.Redirect)
}

const htmlTemplate = `<!doctype html>
<html lang="zh-CN">
  <head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1, shrink-to-fit=no">
	<link href="https://cdn.bootcdn.net/ajax/libs/twitter-bootstrap/4.0.0/css/bootstrap.min.css" rel="stylesheet">
    <title>%s</title>
  </head>
  <body><div class="container">%s</div></body>
</html>`

// Redirect to the target url
func (ctl *InfoController) Redirect(ctx context.Context, webCtx web.Context) web.Response {
	key := webCtx.PathVar("key")
	if key == "" {
		return webCtx.JSONError("invalid key", http.StatusBadRequest)
	}

	url, err := ctl.rds.Get(ctx, fmt.Sprintf("redirect:%s", key)).Result()
	if err != nil {
		return webCtx.JSONError("invalid key", http.StatusBadRequest)
	}

	return webCtx.HTML(fmt.Sprintf(htmlTemplate, "Redirect", fmt.Sprintf(`<div style="margin: 0; text-align: center; margin-top: 50px;"><a href="%s">NSFW</a></div>`, url)))
}

const CurrentVersion = "2.0.0"

// VersionCheck version check
func (ctl *InfoController) VersionCheck(ctx web.Context) web.Response {
	clientVersion := ctx.Input("version")
	clientOS := ctx.Input("os")

	var hasUpdate bool
	if clientOS == "android" || clientOS == "macos" {
		hasUpdate = misc.VersionNewer(CurrentVersion, clientVersion)
	}

	return ctx.JSON(web.M{
		"has_update":     hasUpdate,
		"server_version": CurrentVersion,
		"force_update":   false,
		"url":            "https://aidea.aicode.cc",
		"message":        fmt.Sprintf("新版本 %s 发布啦，赶快去更新吧！", CurrentVersion),
	})
}

// Version return server version
func (ctl *InfoController) Version(ctx web.Context) web.Response {
	return ctx.JSON(web.M{
		"version": CurrentVersion,
	})
}

type Capability struct {
}

// Capabilities get the server's capability list
func (ctl *InfoController) Capabilities(ctx context.Context, webCtx web.Context, user *auth.UserOptional, client *auth.ClientInfo) web.Response {
	cap := Capability{
		// 设置服务器能力列表
	}
	return webCtx.JSON(cap)
}
