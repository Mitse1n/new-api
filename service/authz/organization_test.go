package authz

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestOrganizationPermissionsUseFixedMemberRoles(t *testing.T) {
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
		{model.OrgRoleOwner, "org.token", "read_all", false},
		{model.OrgRoleOwner, "org.token", "write_all", false},
		{model.OrgRoleAdmin, "org.token", "read_all", false},
		{model.OrgRoleAdmin, "org.token", "write_all", false},
		{model.OrgRoleOwner, "org.usage", "read_all", true},
		{model.OrgRoleAdmin, "org.usage", "read_all", true},
		{"billing", "org.billing", "write", false},
		{model.OrgRoleOwner, "channel", "read", false},
	} {
		t.Run(test.role+"/"+test.resource+"/"+test.action, func(t *testing.T) {
			assert.Equal(t, test.allowed, CanOrg(1, 10, test.role, Permission{Resource: test.resource, Action: test.action}))
		})
	}
	permission := Permission{Resource: "org.token", Action: "write"}
	assert.False(t, CanOrg(1, 0, model.OrgRoleOwner, permission))
	assert.False(t, CanOrg(0, 10, model.OrgRoleOwner, permission))
	assert.False(t, CanOrg(1, -1, model.OrgRoleOwner, permission))
	assert.False(t, CanOrg(1, 10, "root", permission))
	assert.Equal(t, PermissionsMap{
		"org.member":       {"read": true, "write": false},
		"org.token":        {"read": true, "write": true},
		"org.usage":        {"read": true, "read_all": false},
		"org.billing":      {"read": false, "write": false},
		"org.subscription": {"purchase": false},
		"org.settings":     {"read": false, "write": false},
		"org.lifecycle":    {"write": false},
	}, OrganizationCapabilities(1, 10, model.OrgRoleMember))
}

func TestOrganizationInitPreservesPlatformOverridesAcrossRestart(t *testing.T) {
	db := newAuthzTestDB(t)
	legacy := model.CasbinRule{Ptype: "p", V0: UserSubject(2), V1: "channel", V2: "read", V3: EffectDeny}
	require.NoError(t, db.Create(&legacy).Error)
	for i := 0; i < 2; i++ {
		require.NoError(t, Init(db))
		assert.False(t, Can(2, common.RoleAdminUser, ChannelRead))
		assert.True(t, Can(3, common.RoleAdminUser, ChannelRead))
	}
	require.NoError(t, db.First(&legacy, legacy.Id).Error)
	assert.Equal(t, "channel", legacy.V1)
	assert.Equal(t, "read", legacy.V2)
	assert.Equal(t, EffectDeny, legacy.V3)
	assert.Empty(t, legacy.V4)
}

func TestOrganizationPermissionsAreIndependentOfPlatformOverrides(t *testing.T) {
	db := newAuthzTestDB(t)
	require.NoError(t, Init(db))
	for _, resource := range Catalog() {
		assert.False(t, strings.HasPrefix(resource.Resource, "org."))
	}
	input := PermissionsMap{"org.billing": {"write": true}, "channel": {"write": false}}
	require.NoError(t, SetUserPermissions(42, input))
	assert.NotContains(t, ExplicitUserOverrides(42), "org.billing")
	assert.False(t, Can(42, common.RoleAdminUser, ChannelWrite))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return SetUserPermissionsInTx(tx, 43, input) }))
	require.NoError(t, ReloadPolicy())
	assert.NotContains(t, ExplicitUserOverrides(43), "org.billing")
	var organizationOverrides int64
	require.NoError(t, db.Model(&model.CasbinRule{}).Where("v0 IN ? AND v1 = ?", []string{UserSubject(42), UserSubject(43)}, "org.billing").Count(&organizationOverrides).Error)
	assert.Zero(t, organizationOverrides, "platform writes must not persist organization permissions")
	assert.NotContains(t, Capabilities(1, common.RoleRootUser), "org.token")
	assert.False(t, CanOrg(42, 10, model.OrgRoleMember, Permission{Resource: "org.billing", Action: "write"}))
	require.NoError(t, SetUserPermissions(43, PermissionsMap{"org.billing": {"write": false}}))
	assert.True(t, CanOrg(43, 10, model.OrgRoleAdmin, Permission{Resource: "org.billing", Action: "write"}))
}
