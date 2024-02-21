package rate

import (
	"context"
	"errors"
	"github.com/go-redis/redis_rate/v10"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrRateLimitExceeded = errors.New("request frequency is too high, please try again later")
var ErrDailyFreeLimitExceeded = errors.New("exceeding daily free times")

func NewLimiter(rdb *redis.Client) *redis_rate.Limiter {
	return redis_rate.NewLimiter(rdb)
}

func MaxRequestsInPeriod(count int, period time.Duration) redis_rate.Limit {
	return redis_rate.Limit{Rate: count, Burst: count, Period: period}
}

type Limiter struct {
	limiter *redis_rate.Limiter
	rds     *redis.Client
}

func New(rds *redis.Client, limiter *redis_rate.Limiter) *Limiter {
	return &Limiter{limiter: limiter, rds: rds}
}

// Allow check if access is allowed
func (rl *Limiter) Allow(ctx context.Context, key string, limit redis_rate.Limit) error {
	res, err := rl.limiter.Allow(ctx, key, limit)
	if err != nil {
		return err
	}

	if res.Remaining <= 0 {
		return ErrRateLimitExceeded
	}

	return nil
}

// OperationCount get the number of operations
func (rl *Limiter) OperationCount(ctx context.Context, key string) (int64, error) {
	res, err := rl.rds.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, nil
		}

		return 0, err
	}

	if res == "" {
		return 0, nil
	}

	return strconv.ParseInt(res, 10, 64)
}

// OperationIncr increase number of operations
func (rl *Limiter) OperationIncr(ctx context.Context, key string, ttl time.Duration) error {
	_, err := rl.rds.Incr(ctx, key).Result()
	if err != nil {
		return err
	}

	_, err = rl.rds.Expire(ctx, key, ttl).Result()
	return err
}
