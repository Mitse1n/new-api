package model

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrganizationInviteBindsUsernameToAccountWithoutEmail(t *testing.T) {
	db := organizationTestDatabase(t)
	users := []User{{Username: "owner", AffCode: "owner"}, {Username: "Member", AffCode: "member"}, {Username: "outsider", Email: "Member", AffCode: "outsider"}, {Username: "disabled", Status: common.UserStatusDisabled, AffCode: "disabled"}}
	require.NoError(t, db.Create(&users).Error)
	org, err := CreateTeamOrganization(users[0].Id, "Team", "username-team")
	require.NoError(t, err)
	for _, username := range []string{"unknown", "member", "disabled"} {
		_, err = CreateOrganizationInvite(org.Id, users[0].Id, username, OrgRoleMember)
		assert.ErrorIs(t, err, ErrOrganizationInviteUser, username)
	}
	_, err = CreateOrganizationInvite(org.Id, users[0].Id, "  ", OrgRoleMember)
	assert.ErrorIs(t, err, ErrOrganizationInput)
	invite, err := CreateOrganizationInvite(org.Id, users[0].Id, " Member ", OrgRoleMember)
	require.NoError(t, err)
	assert.Equal(t, users[1].Id, invite.InviteeId)
	assert.Equal(t, "Member", invite.Username)
	_, err = CreateOrganizationInvite(org.Id, users[0].Id, "Member", OrgRoleMember)
	assert.ErrorIs(t, err, ErrOrganizationInvitePending)
	_, err = AcceptOrganizationInvite(users[2].Id, invite.Id)
	assert.ErrorIs(t, err, ErrOrganizationInvite)
	// A later rename cannot hand an existing invitation to the new holder of that name.
	require.NoError(t, db.Model(&users[1]).Update("username", "renamed").Error)
	require.NoError(t, db.Model(&users[2]).Update("username", "Member").Error)
	_, err = AcceptOrganizationInvite(users[2].Id, invite.Id)
	assert.ErrorIs(t, err, ErrOrganizationInvite)
	_, err = CreateOrganizationInvite(org.Id, users[0].Id, "renamed", OrgRoleMember)
	assert.ErrorIs(t, err, ErrOrganizationInvitePending)
	resent, err := ResendOrganizationInvite(org.Id, users[0].Id, invite.Id)
	require.NoError(t, err)
	assert.Equal(t, "renamed", resent.Username)
	require.NoError(t, db.Model(&users[1]).Update("status", common.UserStatusDisabled).Error)
	_, err = AcceptOrganizationInvite(users[1].Id, invite.Id)
	assert.ErrorIs(t, err, ErrOrganizationInvite)
	require.NoError(t, db.Model(&users[1]).Update("status", common.UserStatusEnabled).Error)
	accepted, err := AcceptOrganizationInvite(users[1].Id, invite.Id)
	require.NoError(t, err)
	assert.Equal(t, org.Id, accepted)
	_, err = AcceptOrganizationInvite(users[1].Id, invite.Id)
	require.NoError(t, err)
	_, err = CreateOrganizationInvite(org.Id, users[0].Id, "renamed", OrgRoleMember)
	assert.ErrorIs(t, err, ErrOrganizationMemberExists)
}

// The exact invitation schema deployed before username invitations (222b538f).
type emailOrganizationInvite struct {
	Id         int    `json:"id"`
	OrgId      int    `gorm:"index:idx_org_invite;not null"`
	Email      string `gorm:"type:varchar(254);not null"`
	Role       string `gorm:"type:varchar(16);not null"`
	TokenHash  string `gorm:"type:char(64);uniqueIndex;not null"`
	Status     string `gorm:"type:varchar(16);not null"`
	InviterId  int
	AcceptedBy int
	ExpiresAt  int64 `gorm:"type:bigint"`
	CreatedAt  int64 `gorm:"autoCreateTime"`
}

func (emailOrganizationInvite) TableName() string { return "organization_invites" }

func TestOrganizationInviteMigrationPreservesLegacyIdentityAndTokenUniqueness(t *testing.T) {
	db := organizationTestDatabase(t)
	users := []User{{Username: "owner", AffCode: "owner"}, {Username: "member@example.test", Email: "member@example.test", AffCode: "member"}}
	require.NoError(t, db.Create(&users).Error)
	org, err := CreateTeamOrganization(users[0].Id, "Team", "legacy-team")
	require.NoError(t, err)
	require.NoError(t, db.Migrator().DropTable(&OrganizationInvite{}))
	require.NoError(t, db.AutoMigrate(&emailOrganizationInvite{}))
	token := strings.Repeat("a", 64)
	hash := sha256.Sum256([]byte(token))
	legacy := emailOrganizationInvite{OrgId: org.Id, Email: users[1].Email, Role: OrgRoleMember, TokenHash: hex.EncodeToString(hash[:]), Status: "pending", InviterId: users[0].Id, ExpiresAt: common.GetTimestamp() + 3600}
	require.NoError(t, db.Create(&legacy).Error)
	for i := 0; i < 2; i++ {
		require.NoError(t, db.AutoMigrate(&OrganizationInvite{}))
	}
	var migrated OrganizationInvite
	require.NoError(t, db.First(&migrated, legacy.Id).Error)
	assert.Equal(t, legacy.Email, migrated.Email)
	assert.Equal(t, legacy.TokenHash, migrated.TokenHash)
	assert.Equal(t, legacy.ExpiresAt, migrated.ExpiresAt)
	assert.Equal(t, "pending", migrated.Status)
	assert.Zero(t, migrated.InviteeId)
	assert.Empty(t, migrated.Username)
	assert.True(t, db.Migrator().HasIndex(&OrganizationInvite{}, "idx_org_invite"))
	assert.True(t, db.Migrator().HasIndex(&OrganizationInvite{}, "idx_org_invite_recipient"))
	duplicate := migrated
	duplicate.Id = 0
	assert.Error(t, db.Create(&duplicate).Error)
	_, err = AcceptOrganizationInvite(users[1].Id, legacy.Id)
	assert.ErrorIs(t, err, ErrOrganizationInvite)
	_, err = ResendOrganizationInvite(org.Id, users[0].Id, legacy.Id)
	assert.ErrorIs(t, err, ErrOrganizationInviteLegacy)
	require.NoError(t, RevokeOrganizationInvite(org.Id, users[0].Id, legacy.Id))
	replacement, err := CreateOrganizationInvite(org.Id, users[0].Id, users[1].Username, OrgRoleMember)
	require.NoError(t, err)
	_, err = AcceptOrganizationInvite(users[1].Id, replacement.Id)
	require.NoError(t, err)
}

func TestOrganizationInvitationInboxIsAccountScopedAndRequiresConsent(t *testing.T) {
	db := organizationTestDatabase(t)
	users := []User{{Username: "owner", AffCode: "owner"}, {Username: "recipient", AffCode: "recipient"}, {Username: "other", AffCode: "other"}}
	require.NoError(t, db.Create(&users).Error)
	org, err := CreateTeamOrganization(users[0].Id, "Inbox Team", "inbox-team")
	require.NoError(t, err)
	invite, err := CreateOrganizationInvite(org.Id, users[0].Id, users[1].Username, OrgRoleMember)
	require.NoError(t, err)
	inbox, err := ListIncomingOrganizationInvites(users[1].Id)
	require.NoError(t, err)
	require.Len(t, inbox, 1)
	assert.Equal(t, invite.Id, inbox[0].Id)
	assert.Equal(t, org.Name, inbox[0].OrganizationName)
	assert.Equal(t, "owner", inbox[0].InviterUsername)
	_, _, err = GetOrganizationMembership(org.Id, users[1].Id)
	assert.ErrorIs(t, err, ErrOrganizationAccess, "sending an invitation must not add a member")
	others, err := ListIncomingOrganizationInvites(users[2].Id)
	require.NoError(t, err)
	assert.Empty(t, others)
	assert.ErrorIs(t, DeclineOrganizationInvite(users[2].Id, invite.Id), ErrOrganizationInvite)
	_, err = AcceptOrganizationInvite(users[2].Id, invite.Id)
	assert.ErrorIs(t, err, ErrOrganizationInvite)
	require.NoError(t, DeclineOrganizationInvite(users[1].Id, invite.Id))
	require.NoError(t, DeclineOrganizationInvite(users[1].Id, invite.Id), "declining is idempotent")
	inbox, err = ListIncomingOrganizationInvites(users[1].Id)
	require.NoError(t, err)
	assert.Empty(t, inbox)
	_, err = AcceptOrganizationInvite(users[1].Id, invite.Id)
	assert.ErrorIs(t, err, ErrOrganizationInvite)
	replacement, err := CreateOrganizationInvite(org.Id, users[0].Id, users[1].Username, OrgRoleMember)
	require.NoError(t, err)
	require.NoError(t, RevokeOrganizationInvite(org.Id, users[0].Id, replacement.Id))
	_, err = AcceptOrganizationInvite(users[1].Id, replacement.Id)
	assert.ErrorIs(t, err, ErrOrganizationInvite)
	replacement, err = CreateOrganizationInvite(org.Id, users[0].Id, users[1].Username, OrgRoleMember)
	require.NoError(t, err)
	require.NoError(t, db.Model(replacement).Update("expires_at", common.GetTimestamp()-1).Error)
	inbox, err = ListIncomingOrganizationInvites(users[1].Id)
	require.NoError(t, err)
	assert.Empty(t, inbox)
	_, err = AcceptOrganizationInvite(users[1].Id, replacement.Id)
	assert.ErrorIs(t, err, ErrOrganizationInvite)
	_, err = ResendOrganizationInvite(org.Id, users[0].Id, replacement.Id)
	require.NoError(t, err)
	inbox, err = ListIncomingOrganizationInvites(users[1].Id)
	require.NoError(t, err)
	require.Len(t, inbox, 1)
	require.NoError(t, db.Model(org).Update("status", OrganizationDisabled).Error)
	inbox, err = ListIncomingOrganizationInvites(users[1].Id)
	require.NoError(t, err)
	assert.Empty(t, inbox)
	_, err = AcceptOrganizationInvite(users[1].Id, replacement.Id)
	assert.ErrorIs(t, err, ErrOrganizationInvite)
	require.NoError(t, db.Model(org).Update("status", OrganizationActive).Error)
	_, err = AcceptOrganizationInvite(users[1].Id, replacement.Id)
	require.NoError(t, err)
	inbox, err = ListIncomingOrganizationInvites(users[1].Id)
	require.NoError(t, err)
	assert.Empty(t, inbox)
	assert.ErrorIs(t, DeclineOrganizationInvite(users[1].Id, replacement.Id), ErrOrganizationInvite)
	var declinedAudit OrganizationAudit
	require.NoError(t, db.Where("org_id = ? AND actor_id = ? AND action = ?", org.Id, users[1].Id, "member.decline").First(&declinedAudit).Error)
}
