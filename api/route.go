package api

import (
	"github.com/gorilla/mux"
	"github.com/mylxsw/aidea-chat-server/api/controllers"
	"github.com/mylxsw/aidea-chat-server/config"
	"github.com/mylxsw/glacier/infra"
	"github.com/mylxsw/glacier/web"
	"net/http"
)

// 需要鉴权的 URLs
var needAuthPrefix = []string{
	"/v1/users", // 用户管理
	"/v1/tasks", // 任务管理

	"/v1/auth/bind-phone",  // 绑定手机号码
	"/v1/auth/bind-wechat", // 绑定微信
}

func routes(resolver infra.Resolver, r *web.MiddlewareRouter) {
	r.Controllers(
		"/v1",
		controllers.NewInfoController(resolver),
		controllers.NewAuthController(resolver),
		controllers.NewAppleAuthController(resolver),
		controllers.NewTaskController(resolver),
		controllers.NewUserController(resolver),
		controllers.NewChatController(resolver),
	)
}

func muxRoutes(resolver infra.Resolver, router *mux.Router) {
	resolver.MustResolve(func(conf *config.Config) {
		// add prometheus metrics support
		router.PathPrefix("/metrics").Handler(PrometheusHandler{token: conf.PrometheusToken})
		// add health check interface support
		router.PathPrefix("/health").Handler(HealthCheck{})
		// universal Links
		router.PathPrefix("/.well-known/apple-app-site-association").HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writer.Header().Add("Content-Type", "application/json")

			data := `{"applinks":{"apps":[],"details":[{"appID":"N95437SZ2A.cc.aicode.flutter.askaide.askaide","paths":["/wechat-login/*","/wechat-links/*"]}]}}`
			if conf.UniversalLinkConfig != "" {
				data = conf.UniversalLinkConfig
			}

			_, _ = writer.Write([]byte(data))
		})
	})
}
