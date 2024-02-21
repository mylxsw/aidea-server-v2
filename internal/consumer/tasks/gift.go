package tasks

import (
	"context"
	"github.com/mylxsw/aidea-chat-server/internal/coins"
	"github.com/mylxsw/aidea-chat-server/pkg/repo"
	"github.com/mylxsw/asteria/log"
	"time"
)

// inviteGiftHandler 引荐奖励
func inviteGiftHandler(ctx context.Context, rp *repo.Repository, userId, invitedByUserId int64) {
	// 引荐人奖励
	if coins.InviteGiftCoins > 0 {
		if _, err := rp.Quota.AddUserQuota(ctx, invitedByUserId, int64(coins.InviteGiftCoins), time.Now().AddDate(0, 1, 0), "引荐奖励", ""); err != nil {
			log.WithFields(log.Fields{"user_id": invitedByUserId}).Errorf("create user quota failed: %s", err)
		}
	}

	// 被引荐人奖励
	if coins.InvitedGiftCoins > 0 {
		if _, err := rp.Quota.AddUserQuota(ctx, userId, int64(coins.InvitedGiftCoins), time.Now().AddDate(0, 1, 0), "引荐注册奖励", ""); err != nil {
			log.WithFields(log.Fields{"user_id": userId}).Errorf("create user quota failed: %s", err)
		}
	}
}
