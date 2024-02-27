package api

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-redis/redis_rate/v10"
	"github.com/mylxsw/aidea-chat-server/api/auth"
	"github.com/mylxsw/aidea-chat-server/config"
	"github.com/mylxsw/aidea-chat-server/pkg/jwt"
	"github.com/mylxsw/aidea-chat-server/pkg/rate"
	"github.com/mylxsw/aidea-chat-server/pkg/repo"
	"github.com/mylxsw/aidea-chat-server/pkg/service"
	"github.com/mylxsw/asteria/log"
	"github.com/mylxsw/glacier/infra"
	"github.com/mylxsw/glacier/listener"
	"github.com/mylxsw/glacier/web"
	"github.com/mylxsw/go-utils/str"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

var ErrUserDestroyed = errors.New("user is destroyed")

type Provider struct{}

// Aggregates 实现 infra.ProviderAggregate 接口
func (Provider) Aggregates() []infra.Provider {
	return []infra.Provider{
		web.Provider(
			listener.FlagContext("listen"),
			web.SetRouteHandlerOption(buildRouter),
			web.SetMuxRouteHandlerOption(muxRoutes),
			web.SetExceptionHandlerOption(exceptionHandler),
			web.SetIgnoreLastSlashOption(true),
		),
	}
}

// Register 实现 infra.Provider 接口
func (Provider) Register(binder infra.Binder) {}

// exceptionHandler 异常处理器
func exceptionHandler(ctx web.Context, err interface{}) web.Response {
	if err == ErrUserDestroyed {
		return ctx.JSONError("Account unavailable: User account has been destroyed", http.StatusForbidden)
	}

	debug.PrintStack()

	log.Errorf("request %s failed: %v, stack is %s", ctx.Request().Raw().URL.Path, err, string(debug.Stack()))
	return ctx.JSONWithCode(web.M{"error": fmt.Sprintf("%v", err)}, http.StatusInternalServerError)
}

// buildRouter 注册路由规则
func buildRouter(resolver infra.Resolver, router web.Router, mw web.RequestMiddleware) {
	conf := resolver.MustGet((*config.Config)(nil)).(*config.Config)

	mws := make([]web.HandlerDecorator, 0)
	// 跨域请求处理
	if conf.EnableCORS {
		mws = append(mws, mw.CORS("*"))
	}

	// Prometheus 监控指标
	reqCounterMetric := BuildCounterVec(
		"aidea",
		"http_request_count",
		"http request counts",
		[]string{"method", "path", "code", "platform"},
	)

	// 添加 web 中间件
	resolver.MustResolve(func(tk *jwt.Token, userSrv *service.UserService, limiter *redis_rate.Limiter) {
		mws = append(mws, func(handler web.WebHandler) web.WebHandler {
			return func(ctx web.Context) web.Response {
				ctx.Response().Header("aidea-global-alert-id", "20231204")
				//ctx.StreamResponse().Header("aidea-global-alert-type", "info")
				//ctx.StreamResponse().Header("aidea-global-alert-pages", "")
				//ctx.StreamResponse().Header("aidea-global-alert-msg", base64.StdEncoding.EncodeToString([]byte("服务器正在维护中，预计 2023 年 11 月 12 日 00:00:00 恢复，[查看详情](https://status.aicode.cc/status/aidea)。")))

				return handler(ctx)
			}
		})
		mws = append(mws, mw.BeforeInterceptor(func(webCtx web.Context) web.Response {
			// 跨域请求处理，OPTIONS 请求直接返回
			if webCtx.Method() == http.MethodOptions {
				return webCtx.JSON(web.M{})
			}

			// 基于客户端 IP 的限流
			clientIP := webCtx.Header("X-Real-IP")
			if clientIP == "" {
				return nil
			}

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			m, err := limiter.Allow(ctx, fmt.Sprintf("request-ip:%s:freq", clientIP), rate.MaxRequestsInPeriod(30, 10*time.Second))
			if err != nil {
				return webCtx.JSONError("rate-limiter: internal server error", http.StatusInternalServerError)
			}

			if m.Remaining <= 0 {
				log.WithFields(log.Fields{"ip": clientIP}).Warningf("client request too frequently")
				return webCtx.JSONError(rate.ErrRateLimitExceeded.Error(), http.StatusTooManyRequests)
			}

			return nil
		}))

		mws = append(mws,
			mw.CustomAccessLog(func(cal web.CustomAccessLog) {
				// 记录访问日志
				platform := readFromWebContext(cal.Context, "platform")
				path, _ := cal.Context.CurrentRoute().GetPathTemplate()
				reqCounterMetric.WithLabelValues(
					cal.Method,
					path,
					strconv.Itoa(cal.ResponseCode),
					platform,
				).Inc()

				log.F(log.M{
					"method":   cal.Method,
					"url":      cal.URL,
					"code":     cal.ResponseCode,
					"elapse":   cal.Elapse.Milliseconds(),
					"ip":       cal.Context.Header("X-Real-IP"),
					"lang":     readFromWebContext(cal.Context, "language"),
					"ver":      readFromWebContext(cal.Context, "client-version"),
					"plat":     platform,
					"plat-ver": readFromWebContext(cal.Context, "platform-version"),
				}).Debug("request")
			}),
			authHandler(
				func(webCtx web.Context, credential string) error {
					urlPath := webCtx.Request().Raw().URL.Path
					needAuth := str.HasPrefixes(urlPath, needAuthPrefix)

					claims, err := tk.ParseToken(credential)
					if needAuth && err != nil {
						return errors.New("invalid auth credential")
					}

					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()

					// 查询用户信息
					var user *auth.User
					if u, err := userSrv.GetUserByID(ctx, claims.Int64Value("id"), false); err != nil {
						if needAuth {
							if errors.Is(err, repo.ErrNotFound) {
								return errors.New("invalid auth credential, user not found")
							}

							return err
						}
					} else {
						if u.Status == repo.UserStatusDeleted {
							if needAuth {
								return ErrUserDestroyed
							}

							u = nil
						}

						user = auth.CreateAuthUserFromModel(u)
					}

					if needAuth {
						if user == nil {
							return errors.New("invalid auth credential, user not found")
						}

						// // 请求限流(基于用户 ID)
						// ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
						// defer cancel()

						// m, err := limiter.Allow(ctx, fmt.Sprintf("request:%d:freq", claims.Int64Value("id")), rate.MaxRequestsInPeriod(10, 1*time.Minute))
						// if err != nil {
						// 	return errors.New("rate-limiter: interapi server error")
						// }

						// if m.Remaining <= 0 {
						// 	return errors.New("request frequency is too high, please try again later")
						// }

						webCtx.Provide(func() *auth.User { return user })
						webCtx.Provide(func() *auth.UserOptional {
							return &auth.UserOptional{User: user}
						})
					} else {
						webCtx.Provide(func() *auth.UserOptional { return &auth.UserOptional{User: user} })
					}

					return nil
				},
				func(ctx web.Context) bool {
					// inject client information
					ctx.Provide(func() *auth.ClientInfo {
						return &auth.ClientInfo{
							Version:         readFromWebContext(ctx, "client-version"),
							Platform:        readFromWebContext(ctx, "platform"),
							PlatformVersion: readFromWebContext(ctx, "platform-version"),
							Language:        readFromWebContext(ctx, "language"),
							IP:              ctx.Header("X-Real-IP"),
						}
					})

					// URL that must be authenticated
					needAuth := str.HasPrefixes(ctx.Request().Raw().URL.Path, needAuthPrefix)
					if needAuth {
						return false
					}

					authHeader := strings.ToLower(readFromWebContext(ctx, "authorization"))
					// If there is an Authorization header and the Authorization header starts with Bearer,
					// authentication is required.
					if strings.HasPrefix(authHeader, "bearer ") {
						return false
					}

					ctx.Provide(func() *auth.UserOptional { return &auth.UserOptional{User: nil} })
					return true
				},
			),
		)
	})

	// 注册控制器，所有的控制器 API 都以 `/server` 作为接口前缀
	r := router.WithMiddleware(mws...)
	routes(resolver, r)
}

// readFromWebContext read the request parameters first.
// If the request parameters do not exist, read the request header.
func readFromWebContext(webCtx web.Context, key string) string {
	val := webCtx.Input(key)
	if val != "" {
		if strings.ToLower(key) == "authorization" {
			return "Bearer " + val
		}

		return val
	}

	val = webCtx.Header(strings.ToUpper(key))
	if val != "" {
		return val
	}

	return webCtx.Header("X-" + strings.ToUpper(key))
}

func authHandler(cb func(ctx web.Context, credential string) error, skip func(ctx web.Context) bool) web.HandlerDecorator {
	return func(handler web.WebHandler) web.WebHandler {
		return func(ctx web.Context) (resp web.Response) {
			if !skip(ctx) {
				authHeader := readFromWebContext(ctx, "authorization")
				segs := strings.SplitN(authHeader, " ", 2)

				var authToken string
				if len(segs) >= 2 {
					if segs[0] != "Bearer" {
						return ctx.JSONError("auth failed: invalid auth type", http.StatusUnauthorized)
					}
					authToken = segs[1]
				} else {
					authToken = segs[0]
				}

				if err := cb(ctx, authToken); err != nil {
					return ctx.JSONError(fmt.Sprintf("auth failed: %s", err), http.StatusUnauthorized)
				}
			}

			return handler(ctx)
		}
	}
}
