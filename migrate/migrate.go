package migrate

import (
	"context"
	"database/sql"
	"github.com/mylxsw/aidea-chat-server/migrate/data"
	"github.com/mylxsw/asteria/log"
	"github.com/mylxsw/eloquent/migrate"
	"time"
)

func Migrate(ctx context.Context, db *sql.DB) error {
	log.Debugf("database migration in progress")
	startTs := time.Now()
	defer func() {
		log.Debugf("database migration execution is completed and takes time %s", time.Since(startTs).String())
	}()

	m := migrate.NewManager(db).Init(ctx)

	data.Migrate20240221(m)

	return m.Run(ctx)
}
