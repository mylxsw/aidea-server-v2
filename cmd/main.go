package main

import (
	"context"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"github.com/mylxsw/aidea-chat-server/api"
	"github.com/mylxsw/aidea-chat-server/config"
	"github.com/mylxsw/aidea-chat-server/internal/consumer"
	"github.com/mylxsw/aidea-chat-server/internal/queue"
	"github.com/mylxsw/aidea-chat-server/migrate"
	"github.com/mylxsw/aidea-chat-server/pkg/mail"
	"github.com/mylxsw/aidea-chat-server/pkg/proxy"
	"github.com/mylxsw/aidea-chat-server/pkg/rate"
	"github.com/mylxsw/aidea-chat-server/pkg/redis"
	"github.com/mylxsw/aidea-chat-server/pkg/repo"
	"github.com/mylxsw/aidea-chat-server/pkg/service"
	"github.com/mylxsw/aidea-chat-server/pkg/token"
	"github.com/mylxsw/aidea-chat-server/pkg/wechat"
	"github.com/mylxsw/asteria/formatter"
	"github.com/mylxsw/asteria/level"
	"github.com/mylxsw/asteria/log"
	"github.com/mylxsw/asteria/writer"
	"github.com/mylxsw/glacier/infra"
	"github.com/mylxsw/glacier/starter/app"
	"path/filepath"
	"time"
)

var (
	Version   string
	GitCommit string
)

func main() {
	// turn off framework WARN log
	infra.WARN = false

	ins := app.Create(fmt.Sprintf("%s(%s)", Version, GitCommit), 3).WithYAMLFlag("conf")

	// load configurations
	config.Register(ins)

	// log configuration
	ins.Init(func(f infra.FlagContext) error {
		if !f.Bool("log-color") {
			log.All().LogFormatter(formatter.NewJSONFormatter())
		}

		if f.String("log-path") != "" {
			log.All().LogWriter(writer.NewDefaultRotatingFileWriter(context.TODO(), func(le level.Level, module string) string {
				return filepath.Join(f.String("log-path"), fmt.Sprintf("%s.%s.log", le.GetLevelName(), time.Now().Format("20060102")))
			}))
		}

		startDelay := f.Duration("start-delay")
		if startDelay > 0 {
			log.Infof("service starts after delay %s", startDelay)
			time.Sleep(startDelay)
		}

		return nil
	})

	//ins.Async(func(conf *config.Config) {
	//	log.With(conf).Debugf("configuration loaded")
	//})

	ins.OnServerReady(func(f infra.FlagContext) {
		log.Infof("service started and listening on %s", f.String("listen"))
	})

	// configure the service module to be loaded
	ins.Provider(
		migrate.Provider{},
		redis.Provider{},
		rate.Provider{},
		repo.Provider{},
		service.Provider{},
		api.Provider{},
		token.Provider{},
		wechat.Provider{},
		mail.Provider{},
		queue.Provider{},
		consumer.Provider{},
		proxy.Provider{},
	)

	app.MustRun(ins)
}
