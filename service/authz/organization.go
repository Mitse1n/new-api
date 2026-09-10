package authz

import (
	"slices"

	"github.com/QuantumNous/new-api/model"
)

func init() {
	RegisterResource(ResourceDefinition{Resource: "organization", LabelKey: "Organizations", Actions: []ActionDefinition{
		{Action: ActionRead, LabelKey: "View organizations", DefaultRoles: []string{BuiltInRoleAdmin}},
		{Action: ActionWrite, LabelKey: "Manage organizations", DefaultRoles: []string{BuiltInRoleAdmin}},
	}})
}

// Organization permissions are fixed product roles, independent of platform
// Casbin policies. Unknown resources, actions and roles have no grants.
var organizationPermissions = map[Permission][]string{
	{Resource: "org.member", Action: "read"}:           {model.OrgRoleOwner, model.OrgRoleAdmin, model.OrgRoleMember},
	{Resource: "org.member", Action: "write"}:          {model.OrgRoleOwner, model.OrgRoleAdmin},
	{Resource: "org.token", Action: "read"}:            {model.OrgRoleOwner, model.OrgRoleAdmin, model.OrgRoleMember},
	{Resource: "org.token", Action: "write"}:           {model.OrgRoleOwner, model.OrgRoleAdmin, model.OrgRoleMember},
	{Resource: "org.usage", Action: "read"}:            {model.OrgRoleOwner, model.OrgRoleAdmin, model.OrgRoleMember},
	{Resource: "org.usage", Action: "read_all"}:        {model.OrgRoleOwner, model.OrgRoleAdmin},
	{Resource: "org.billing", Action: "read"}:          {model.OrgRoleOwner, model.OrgRoleAdmin},
	{Resource: "org.billing", Action: "write"}:         {model.OrgRoleOwner, model.OrgRoleAdmin},
	{Resource: "org.subscription", Action: "purchase"}: {model.OrgRoleOwner, model.OrgRoleAdmin},
	{Resource: "org.settings", Action: "read"}:         {model.OrgRoleOwner, model.OrgRoleAdmin},
	{Resource: "org.settings", Action: "write"}:        {model.OrgRoleOwner, model.OrgRoleAdmin},
	{Resource: "org.lifecycle", Action: "write"}:       {model.OrgRoleOwner},
}

// CanOrg requires a role from a freshly validated membership. The IDs guard
// missing context; callers remain responsible for checking membership in orgID.
func CanOrg(userID, orgID int, role string, permission Permission) bool {
	return userID > 0 && orgID > 0 && slices.Contains(organizationPermissions[permission], role)
}

func OrganizationCapabilities(userID, orgID int, role string) PermissionsMap {
	result := PermissionsMap{}
	for permission := range organizationPermissions {
		if result[permission.Resource] == nil {
			result[permission.Resource] = map[string]bool{}
		}
		result[permission.Resource][permission.Action] = CanOrg(userID, orgID, role, permission)
	}
	return result
}
