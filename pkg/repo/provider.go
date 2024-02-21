package repo

import (
	"database/sql"
	"errors"
	"fmt"
	"github.com/mylxsw/aidea-chat-server/config"
	"github.com/mylxsw/asteria/log"
	"github.com/mylxsw/eloquent/event"
	"github.com/mylxsw/glacier/infra"
	"time"
)

var (
	ErrNotFound = errors.New("not found")
)

type Provider struct{}

func (Provider) Register(binder infra.Binder) {
	binder.MustSingleton(NewCacheRepo)
	binder.MustSingleton(NewUserRepo)
	binder.MustSingleton(NewEventRepo)
	binder.MustSingleton(NewQuotaRepo)
	binder.MustSingleton(NewQueueRepo)

	// MySQL 数据库连接
	binder.MustSingleton(func(conf *config.Config) (*sql.DB, error) {
		conn, err := sql.Open("mysql", conf.DBURI)
		if err != nil {
			// 第一次连接失败，等待 5 秒后重试
			// docker-compose 模式下，数据库可能还未完全初始化完成
			time.Sleep(time.Second * 5)
			conn, err = sql.Open("mysql", conf.DBURI)
		}

		if err != nil {
			return nil, fmt.Errorf("database connection failed: %w", err)
		}

		return conn, nil
	})

	binder.MustSingleton(func(resolver infra.Resolver) *Repository {
		var repo Repository
		resolver.MustAutoWire(&repo)

		return &repo
	})
}

func (Provider) Boot(resolver infra.Resolver) {
	eventManager := event.NewEventManager(event.NewMemoryEventStore())
	event.SetDispatcher(eventManager)

	resolver.MustResolve(func(conf *config.Config) {
		if !conf.DebugWithSQL {
			return
		}

		eventManager.Listen(func(evt event.QueryExecutedEvent) {
			log.WithFields(log.Fields{
				"sql":      evt.SQL,
				"bindings": evt.Bindings,
				"elapse":   evt.Time.String(),
			}).Debugf("database query executed")
		})
	})
}

type Repository struct {
	Cache *CacheRepo `autowire:"@"`
	User  *UserRepo  `autowire:"@"`
	Event *EventRepo `autowire:"@"`
	Quota *QuotaRepo `autowire:"@"`
	Queue *QueueRepo `autowire:"@"`
}
