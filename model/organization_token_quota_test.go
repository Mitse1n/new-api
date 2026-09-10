package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenAdmissionQuotaParity(t *testing.T) {
	for _, orgID := range []int{0, 1} {
		for _, cached := range []bool{false, true} {
			t.Run(fmt.Sprintf("org=%d/redis=%t", orgID, cached), func(t *testing.T) {
				db := organizationTestDatabase(t)
				if cached {
					useUserCacheMiniRedis(t)
				}
				for _, tc := range []struct {
					name      string
					quota     int
					unlimited bool
					allowed   bool
				}{
					{"positive", 10, false, true}, {"zero", 0, false, false}, {"negative", -1, false, false}, {"unlimited", 0, true, true},
				} {
					token := Token{OrgId: orgID, UserId: 1, Key: tc.name, Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: tc.quota, UnlimitedQuota: tc.unlimited}
					require.NoError(t, db.Create(&token).Error)
					_, err := ValidateUserToken(token.Key)
					if tc.allowed {
						require.NoError(t, err)
					} else {
						assert.ErrorIs(t, err, ErrTokenInvalid)
					}
					require.NoError(t, db.First(&token, token.Id).Error)
					expected := common.TokenStatusEnabled
					if !cached && !tc.allowed {
						expected = common.TokenStatusExhausted
					}
					assert.Equal(t, expected, token.Status)
				}
			})
		}
	}
}

func TestOrganizationTokenAdmissionUsesCommittedQuota(t *testing.T) {
	db := organizationTestDatabase(t)
	useUserCacheMiniRedis(t)
	token := Token{OrgId: 1, UserId: 1, Key: "committed-quota", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 10}
	require.NoError(t, db.Create(&token).Error)
	_, err := ValidateUserToken(token.Key)
	require.NoError(t, err)
	require.NoError(t, db.Model(&token).Update("remain_quota", 0).Error)
	_, err = ValidateUserToken(token.Key)
	assert.ErrorIs(t, err, ErrTokenInvalid, "cached positive balance must not allow an exhausted key")
	// Replace the cache with the exhausted snapshot, then simulate a committed refund.
	require.NoError(t, invalidateTokenCacheForMutation(token.Key))
	require.NoError(t, common.RDB.Del(t.Context(), getTokenCacheFenceKey(token.Key)).Err())
	_, err = GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	require.NoError(t, db.Model(&token).Update("remain_quota", 10).Error)
	_, err = ValidateUserToken(token.Key)
	require.NoError(t, err, "cached zero balance must not reject a refunded key")
}
