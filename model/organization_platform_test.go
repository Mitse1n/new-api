package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestOrganizationPlatformSuspensionRequiresPlatformRestore(t *testing.T) {
	db, org, users := organizationBillingFixture(t)
	token := Token{OrgId: org.Id, UserId: users[0].Id, Key: "suspended-owner-key", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, InsertOrganizationToken(&token))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return PlatformChangeOrganizationStatusTx(tx, org.Id, 999, OrganizationDisabled, "platform suspension")
	}))
	for _, status := range []int{OrganizationActive, OrganizationDisabled, OrganizationDeleting} {
		assert.ErrorIs(t, ChangeOrganizationStatus(org.Id, users[0].Id, status, org.Slug), ErrOrganizationAccess)
	}
	listed, err := ListUserOrganizations(users[0].Id)
	require.NoError(t, err)
	assert.Empty(t, listed, "platform-suspended teams cannot be selected or self-restored")
	_, _, err = GetOrganizationMembership(org.Id, users[0].Id)
	assert.ErrorIs(t, err, ErrOrganizationAccess)
	require.NoError(t, db.First(&token, token.Id).Error)
	assert.NotEqual(t, OrganizationActive, token.OrgStatus)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return PlatformChangeOrganizationStatusTx(tx, org.Id, 999, OrganizationActive, "platform restore")
	}))
	_, _, err = GetOrganizationMembership(org.Id, users[0].Id)
	require.NoError(t, err)
	require.NoError(t, db.First(&token, token.Id).Error)
	assert.Equal(t, OrganizationActive, token.OrgStatus)
	// Owner-managed suspension still supports self-service restoration.
	require.NoError(t, ChangeOrganizationStatus(org.Id, users[0].Id, OrganizationDisabled, ""))
	require.NoError(t, ChangeOrganizationStatus(org.Id, users[0].Id, OrganizationActive, ""))
}
