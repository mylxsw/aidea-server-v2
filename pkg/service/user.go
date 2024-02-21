package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/mylxsw/aidea-chat-server/config"
	"github.com/mylxsw/aidea-chat-server/pkg/rate"
	"github.com/mylxsw/aidea-chat-server/pkg/repo"
	"github.com/mylxsw/aidea-chat-server/pkg/repo/model"
	"github.com/mylxsw/asteria/log"
	"github.com/mylxsw/go-utils/must"
	"github.com/redis/go-redis/v9"
	"time"
)

type UserService struct {
	repo    *repo.Repository
	rds     *redis.Client
	limiter *rate.Limiter
	conf    *config.Config
}

func NewUserService(conf *config.Config, rp *repo.Repository, rds *redis.Client, limiter *rate.Limiter) *UserService {
	return &UserService{conf: conf, repo: rp, rds: rds, limiter: limiter}
}

// GetUserByID obtain user information based on user ID, with caching (10 minutes)
func (srv *UserService) GetUserByID(ctx context.Context, id int64, forceUpdate bool) (*model.Users, error) {
	// Note: The current cache will be automatically cleared when the user binds their mobile phone number.
	userKey := fmt.Sprintf("user:%d:info", id)

	if !forceUpdate {
		if user, err := srv.rds.Get(ctx, userKey).Result(); err == nil {
			var u model.Users
			if err := json.Unmarshal([]byte(user), &u); err == nil {
				return &u, nil
			}
		}
	}

	user, err := srv.repo.User.GetUserByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if err := srv.rds.SetNX(ctx, userKey, string(must.Must(json.Marshal(user))), 10*time.Minute).Err(); err != nil {
		return nil, err
	}

	return user, nil
}

// CustomConfig get user-defined configuration
func (srv *UserService) CustomConfig(ctx context.Context, userID int64) (*repo.UserCustomConfig, error) {
	return srv.repo.User.CustomConfig(ctx, userID)
}

// UpdateCustomConfig update user-defined configuration
func (srv *UserService) UpdateCustomConfig(ctx context.Context, userID int64, config repo.UserCustomConfig) error {
	return srv.repo.User.UpdateCustomConfig(ctx, userID, config)
}

type UserQuota struct {
	Quota  int64 `json:"quota"`
	Used   int64 `json:"used"`
	Rest   int64 `json:"rest"`
	Frozen int64 `json:"frozen"`
}

// UserQuota get user quota
func (srv *UserService) UserQuota(ctx context.Context, userID int64) (*UserQuota, error) {
	quota, err := srv.repo.Quota.GetUserQuota(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user quota failed: %w", err)
	}

	frozen, err := srv.rds.Get(ctx, srv.userQuotaFrozenCacheKey(userID)).Int()
	if err != nil && !errors.Is(err, redis.Nil) {
		log.F(log.M{"user_id": userID, "quota": quota}).Errorf("failed to query user's frozen quota: %s", err)

		return &UserQuota{Rest: quota.Rest, Quota: quota.Quota, Used: quota.Used}, nil
	}

	return &UserQuota{
		Rest:   quota.Rest,
		Quota:  quota.Quota,
		Used:   quota.Used,
		Frozen: int64(frozen),
	}, nil
}

// FreezeUserQuota freeze user quotas
func (srv *UserService) FreezeUserQuota(ctx context.Context, userID int64, quota int64) error {
	if quota <= 0 {
		return nil
	}

	key := srv.userQuotaFrozenCacheKey(userID)
	_, err := srv.rds.IncrBy(ctx, key, quota).Result()
	if err != nil {
		return fmt.Errorf("freeze user quota failed: %w", err)
	}

	if err := srv.rds.Expire(ctx, key, 5*time.Minute).Err(); err != nil {
		log.F(log.M{"user_id": userID, "quota": quota}).Errorf("failed to set user frozen quota expiration time: %s", err)
	}

	return nil
}

// UnfreezeUserQuota unfreeze user quotas
func (srv *UserService) UnfreezeUserQuota(ctx context.Context, userID int64, quota int64) error {
	if quota <= 0 {
		return nil
	}

	key := srv.userQuotaFrozenCacheKey(userID)
	newVal, err := srv.rds.DecrBy(ctx, key, quota).Result()
	if err != nil {
		return fmt.Errorf("failed to unfreeze user quota: %w", err)
	}

	if newVal <= 0 {
		if err := srv.rds.Del(ctx, key).Err(); err != nil {
			log.F(log.M{"user_id": userID, "quota": quota}).Errorf("failed to clear user frozen quota: %s", err)
		}
	}

	return nil
}

func (srv *UserService) userQuotaFrozenCacheKey(userID int64) string {
	return fmt.Sprintf("user:%d:quota:frozen", userID)
}
