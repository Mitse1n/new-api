package authz

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrganizationPermissionsKeepRolesAndDomainsSeparate(t *testing.T) {
	db := newAuthzTestDB(t)
	require.NoError(t, Init(db))
	for _, test := range []struct {
		role, resource, action string
		allowed                bool
	}{
		{model.OrgRoleOwner, "org.lifecycle", "write", true},
		{model.OrgRoleAdmin, "org.lifecycle", "write", false},
		{model.OrgRoleAdmin, "org.billing", "write", true},
		{model.OrgRoleMember, "org.billing", "write", false},
		{model.OrgRoleMember, "org.token", "write", true},
		{model.OrgRoleMember, "org.token", "write_all", false},
		{"billing", "org.billing", "write", false},
		{model.OrgRoleOwner, "channel", "read", false},
	} {
		t.Run(test.role+"/"+test.resource+"/"+test.action, func(t *testing.T) {
			assert.Equal(t, test.allowed, CanOrg(1, 10, test.role, Permission{Resource: test.resource, Action: test.action}))
		})
	}
	permission := Permission{Resource: "org.token", Action: "write"}
	assert.False(t, CanOrg(1, 0, model.OrgRoleOwner, permission))
	_, err := currentEnforcer().AddPolicy(UserSubject(1), "org:10", permission.Resource, permission.Action, EffectDeny)
	require.NoError(t, err)
	assert.False(t, CanOrg(1, 10, model.OrgRoleOwner, permission))
	assert.True(t, CanOrg(1, 11, model.OrgRoleOwner, permission), "a domain override must not affect another organization")
	assert.NotContains(t, Capabilities(1, common.RoleRootUser), "org.token")
}

func TestOrganizationDomainMigrationPreservesLegacyDenyOnRestart(t *testing.T) {
	db := newAuthzTestDB(t)
	legacy := model.CasbinRule{Ptype: "p", V0: UserSubject(2), V1: "channel", V2: "read", V3: EffectDeny}
	require.NoError(t, db.Create(&legacy).Error)
	for i := 0; i < 2; i++ {
		require.NoError(t, Init(db))
		assert.False(t, Can(2, common.RoleAdminUser, ChannelRead))
		assert.True(t, Can(3, common.RoleAdminUser, ChannelRead))
	}
	require.NoError(t, db.First(&legacy, legacy.Id).Error)
	assert.Equal(t, "*", legacy.V1)
	assert.Equal(t, EffectDeny, legacy.V4)
}
