package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrganizationMemberRevocationDisablesOnlyTheirKeysAndPreservesSettlement(t *testing.T) {
	for _, status := range []int{OrganizationDisabled, OrganizationDeleting} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			db, org, users := organizationBillingFixture(t)
			other, err := CreateTeamOrganization(users[1].Id, "Other", "other-private-keys")
			require.NoError(t, err)
			keys := []Token{
				{OrgId: org.Id, UserId: users[1].Id, Key: "member-revoked", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true},
				{OrgId: org.Id, UserId: users[0].Id, Key: "owner-unaffected", Status: common.TokenStatusEnabled},
				{UserId: users[1].Id, Key: "personal-unaffected", Status: common.TokenStatusEnabled},
				{OrgId: other.Id, UserId: users[1].Id, Key: "other-unaffected", Status: common.TokenStatusEnabled},
			}
			require.NoError(t, db.Create(&keys).Error)
			_, err = ReserveOrganizationRequest(org.Id, users[1].Id, keys[0].Id, "before-disable-settle", 100)
			require.NoError(t, err)
			_, err = ReserveOrganizationRequest(org.Id, users[1].Id, keys[0].Id, "before-disable-refund", 50)
			require.NoError(t, err)
			require.NoError(t, UpdateOrganizationMember(org.Id, users[0].Id, users[1].Id, OrgRoleMember, status, 200))
			for i, key := range keys {
				var saved Token
				require.NoError(t, db.First(&saved, key.Id).Error)
				expected := common.TokenStatusEnabled
				if i == 0 {
					expected = common.TokenStatusDisabled
				}
				assert.Equal(t, expected, saved.Status)
			}
			_, err = ValidateUserToken(keys[0].Key)
			assert.ErrorIs(t, err, ErrTokenInvalid)
			for _, amount := range []int64{0, 10} {
				_, err = ReserveOrganizationCharge(org.Id, users[1].Id, 0, fmt.Sprint("after-disable-", amount), amount)
				assert.ErrorIs(t, err, ErrOrganizationAccess)
			}
			require.NoError(t, FinalizeOrganizationCharge(org.Id, "before-disable-settle", 80, false))
			require.NoError(t, FinalizeOrganizationCharge(org.Id, "before-disable-settle", 80, false))
			require.NoError(t, FinalizeOrganizationCharge(org.Id, "before-disable-refund", 0, true))
			require.NoError(t, db.First(org, org.Id).Error)
			assert.Equal(t, int64(920), org.Quota)
			assert.Equal(t, int64(80), org.UsedQuota)
			require.NoError(t, UpdateOrganizationMember(org.Id, users[0].Id, users[1].Id, OrgRoleMember, OrganizationActive, 200))
			_, err = ValidateUserToken(keys[0].Key)
			assert.ErrorIs(t, err, ErrTokenInvalid, "restoring membership must not revive old keys")
			scope := TokenScope{OrgID: org.Id, UserID: users[1].Id}
			require.NoError(t, UpdateScopedToken(scope, &keys[0], true))
			_, err = ValidateUserToken(keys[0].Key)
			require.NoError(t, err, "active creator can explicitly enable their key")
		})
	}
}
