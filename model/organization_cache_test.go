package model

import (
	"context"
	"errors"
	"net"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrganizationCachedTokenNeedsNoOrganizationLookupAndInvalidatesOnDisable(t *testing.T) {
	db, org, users := organizationBillingFixture(t)
	address := os.Getenv("TENANCY_TEST_REDIS_ADDR")
	if address == "" {
		useUserCacheMiniRedis(t)
	} else {
		oldRDB, oldEnabled := common.RDB, common.RedisEnabled
		common.RDB = redis.NewClient(&redis.Options{Addr: address, DB: 14})
		common.RedisEnabled = true
		require.NoError(t, common.RDB.Ping(context.Background()).Err())
		t.Cleanup(func() { require.NoError(t, common.RDB.Close()); common.RDB, common.RedisEnabled = oldRDB, oldEnabled })
	}
	token := Token{OrgId: org.Id, OrgGroup: "default", OrgStatus: OrganizationActive, OrgSettings: `{"allowed_models":["gpt-4o-mini"]}`, UserId: users[0].Id, Key: "organization-cache-fixture", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, db.Create(&token).Error)
	cacheKeys := []string{getTokenCacheKey(token.Key), getTokenCacheFenceKey(token.Key)}
	require.NoError(t, common.RDB.Del(context.Background(), cacheKeys...).Err())
	t.Cleanup(func() { require.NoError(t, common.RDB.Del(context.Background(), cacheKeys...).Err()) })
	_, err := GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	// A warm authentication cache must be sufficient even when no database
	// handle exists. This fails immediately if organization resolution adds a query.
	DB = nil
	cached, err := GetTokenByKey(token.Key, false)
	DB = db
	require.NoError(t, err)
	assert.Equal(t, org.Id, cached.OrgId)
	assert.Equal(t, token.OrgSettings, cached.OrgSettings)
	require.NoError(t, ChangeOrganizationStatus(org.Id, users[0].Id, OrganizationDisabled, ""))
	refreshed, err := GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	assert.Equal(t, OrganizationDisabled, refreshed.OrgStatus)
}

func TestOrganizationPersonalWalletFallsBackWhenRedisIsUnavailable(t *testing.T) {
	db, _, users := organizationBillingFixture(t)
	org, err := EnsurePersonalOrganization(db, &users[0])
	require.NoError(t, err)
	oldRDB, oldEnabled := common.RDB, common.RedisEnabled
	common.RDB = redis.NewClient(&redis.Options{MaxRetries: -1, Dialer: func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("fixture Redis unavailable")
	}})
	common.RedisEnabled = true
	t.Cleanup(func() { require.NoError(t, common.RDB.Close()); common.RDB, common.RedisEnabled = oldRDB, oldEnabled })
	_, err = ReserveOrganizationCharge(org.Id, users[0].Id, 0, "redis-fallback", 100)
	require.NoError(t, err)
	require.NoError(t, FinalizeOrganizationCharge(org.Id, "redis-fallback", 75, false))
	require.NoError(t, FinalizeOrganizationCharge(org.Id, "redis-fallback", 75, false))
	require.NoError(t, db.First(org, org.Id).Error)
	require.NoError(t, db.First(&users[0], users[0].Id).Error)
	assert.Equal(t, int64(924), org.Quota)
	assert.Equal(t, 924, users[0].Quota)
}
