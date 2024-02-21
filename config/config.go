package config

import (
	"fmt"
	"github.com/mylxsw/aidea-chat-server/pkg/misc"
	"github.com/mylxsw/glacier/infra"
	"github.com/mylxsw/glacier/starter/app"
	"gopkg.in/yaml.v3"
	"os"
)

type Config struct {
	// SessionSecret session encryption key
	SessionSecret string `json:"session_secret,omitempty" yaml:"session_secret,omitempty"`
	// EnableCORS cross-domain support
	EnableCORS bool `json:"enable_cors,omitempty" yaml:"enable_cors,omitempty"`
	// DebugWithSQL whether to enable SQL debugging
	DebugWithSQL bool `json:"debug_with_sql,omitempty" yaml:"debug_with_sql,omitempty"`
	// UniversalLinkConfig universal link configuration
	UniversalLinkConfig string `json:"universal_link_config,omitempty" yaml:"universal_link_config,omitempty"`
	// PrometheusToken Prometheus monitoring token
	PrometheusToken string `json:"prometheus_token,omitempty" yaml:"prometheus_token,omitempty"`

	// DBURI database connection address
	DBURI string `json:"db_uri,omitempty" yaml:"db_uri,omitempty"`
	// Redis
	RedisHost     string `json:"redis_host,omitempty" yaml:"redis_host,omitempty"`
	RedisPort     int    `json:"redis_port,omitempty" yaml:"redis_port,omitempty"`
	RedisPassword string `json:"-" yaml:"redis_password,omitempty"`

	// ProxyURL proxy server address
	ProxyURL string `json:"proxy_url,omitempty" yaml:"proxy_url,omitempty"`

	// QueueWorkers number of task queue workers
	QueueWorkers int `json:"queue_workers,omitempty" yaml:"queue_workers,omitempty"`
	// EnableScheduler whether to enable scheduled task executor
	EnableScheduler bool `json:"enable_scheduler,omitempty" yaml:"enable_scheduler,omitempty"`

	// Mail Email configuration
	Mail Mail `json:"mail,omitempty" yaml:"mail,omitempty"`

	// WeChat configuration
	WeChat WeChat `json:"wechat,omitempty" yaml:"wechat,omitempty"`
	// Apple signIn configuration
	Apple AppleSignIn `json:"apple,omitempty" yaml:"apple,omitempty"`
}

// WeChat configuration
type WeChat struct {
	// AppID WeChat AppID
	AppID string `json:"app_id,omitempty" yaml:"app_id,omitempty"`
	// Secret WeChat Secret
	Secret string `json:"secret,omitempty" yaml:"secret,omitempty"`
}

type AppleSignIn struct {
	TeamID string `json:"team_id" yaml:"team_id"`
	KeyID  string `json:"key_id" yaml:"key_id"`
	Secret string `json:"secret" yaml:"secret"`
}

// RedisAddr returns the address of the Redis server
func (conf *Config) RedisAddr() string {
	return fmt.Sprintf("%s:%d", conf.RedisHost, conf.RedisPort)
}

func (conf *Config) Init() {
	conf.SessionSecret = misc.StringDefault(conf.SessionSecret, "aidea")

	conf.RedisHost = misc.StringDefault(conf.RedisHost, "127.0.0.1")
	conf.RedisPort = misc.IntDefault(conf.RedisPort, 6379)

	conf.QueueWorkers = misc.IntDefault(conf.QueueWorkers, 10)
	conf.EnableScheduler = misc.BoolDefault(conf.EnableScheduler, true)
}

type Mail struct {
	From     string `json:"from,omitempty" yaml:"from,omitempty"`
	Host     string `json:"host,omitempty" yaml:"host,omitempty"`
	Port     int    `json:"port,omitempty" yaml:"port,omitempty"`
	Username string `json:"username,omitempty" yaml:"username,omitempty"`
	Password string `json:"-" yaml:"password,omitempty"`
	UseSSL   bool   `json:"use_ssl,omitempty" yaml:"use_ssl,omitempty"`
}

func Register(ins *app.App) {
	ins.AddStringFlag("listen", ":8080", "Web 服务监听地址")
	ins.AddStringFlag("log-path", "", "log file storage directory, leave blank to write to standard output")
	ins.AddBoolFlag("log-color", "whether to enable colorful logs")
	ins.AddDurationFlag("start-delay", 0, "service start delay")
	ins.AddBoolFlag("disable-migrate", "whether to disable database migration")

	ins.Singleton(func(flg infra.FlagContext) *Config {
		confFilePath := flg.String("conf")
		if confFilePath == "" {
			confFilePath = "config.yaml"
		}

		data, err := os.ReadFile(confFilePath)
		if err != nil {
			panic(fmt.Errorf("read config file failed: %s", err))
		}

		var conf Config
		if err := yaml.Unmarshal(data, &conf); err != nil {
			panic(fmt.Errorf("parse config file failed: %s", err))
		}

		conf.Init()

		return &conf
	})
}
