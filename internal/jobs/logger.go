package jobs

import "github.com/mylxsw/asteria/log"

type cronLogger struct {
}

func (l cronLogger) Info(msg string, keysAndValues ...interface{}) {
	// Just drop it, we don't care
}

func (l cronLogger) Error(err error, msg string, keysAndValues ...interface{}) {
	log.Errorf("[glacier] %s: %v", msg, err)
}
