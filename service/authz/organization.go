package authz

import (
	"fmt"

	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

func migratePolicyDomains(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var rules []model.CasbinRule
		if err := tx.Where("ptype = ? AND (v4 = ? OR v4 IS NULL)", "p", "").Find(&rules).Error; err != nil {
			return err
		}
		for _, rule := range rules {
			effect := rule.V3
			if effect == "" {
				effect = EffectAllow
			}
			if err := tx.Model(&model.CasbinRule{}).Where("id = ?", rule.Id).Updates(map[string]interface{}{
				"v1": "*", "v2": rule.V1, "v3": rule.V2, "v4": effect,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func init() {
	RegisterResource(ResourceDefinition{Scope: "platform", Resource: "organization", LabelKey: "Organizations", Actions: []ActionDefinition{
		{Action: ActionRead, LabelKey: "View organizations", DefaultRoles: []string{BuiltInRoleAdmin}},
		{Action: ActionWrite, LabelKey: "Manage organizations", DefaultRoles: []string{BuiltInRoleAdmin}},
	}})
	for _, spec := range []struct {
		resource string
		actions  []string
	}{
		{"org.member", []string{"read", "write"}},
		{"org.token", []string{"read", "write", "read_all", "write_all"}},
		{"org.usage", []string{"read", "read_all"}},
		{"org.billing", []string{"read", "write"}},
		{"org.subscription", []string{"purchase"}},
		{"org.settings", []string{"read", "write"}},
		{"org.lifecycle", []string{"write"}},
	} {
		resource := ResourceDefinition{Scope: "org", Resource: spec.resource, LabelKey: spec.resource}
		for _, action := range spec.actions {
			resource.Actions = append(resource.Actions, ActionDefinition{Action: action, LabelKey: action})
		}
		RegisterResource(resource)
	}
}

func seedOrganizationPolicies() error {
	e := currentEnforcer()
	for _, role := range []string{model.OrgRoleOwner, model.OrgRoleAdmin, model.OrgRoleMember} {
		for _, resource := range registry {
			if resource.Scope != "org" {
				continue
			}
			for _, action := range resource.Actions {
				allowed := role == model.OrgRoleOwner || role == model.OrgRoleAdmin
				if resource.Resource == "org.lifecycle" {
					allowed = role == model.OrgRoleOwner
				}
				if role == model.OrgRoleMember {
					allowed = (resource.Resource == "org.token" && (action.Action == "read" || action.Action == "write")) ||
						(resource.Resource == "org.usage" && action.Action == "read") ||
						(resource.Resource == "org.member" && action.Action == "read")
				}
				if allowed {
					if _, err := e.AddPolicy("org-role:"+role, "*", resource.Resource, action.Action, EffectAllow); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// CanOrg takes a role from a freshly validated membership. Platform roles do
// not implicitly grant access through ordinary organization endpoints.
func CanOrg(userID, orgID int, role string, permission Permission) bool {
	if userID <= 0 || orgID <= 0 || !isKnownPermission(permission) {
		return false
	}
	if role != model.OrgRoleOwner && role != model.OrgRoleAdmin && role != model.OrgRoleMember {
		return false
	}
	e := currentEnforcer()
	if e == nil {
		return false
	}
	domain := fmt.Sprintf("org:%d", orgID)
	policies, err := e.GetFilteredPolicy(0, UserSubject(userID), domain, permission.Resource, permission.Action)
	if err != nil {
		return false
	}
	for _, policy := range policies {
		if policyEffect(policy) == EffectDeny {
			return false
		}
	}
	allowed, err := e.Enforce("org-role:"+role, domain, permission.Resource, permission.Action)
	return err == nil && allowed
}

func OrganizationCapabilities(userID, orgID int, role string) PermissionsMap {
	result := PermissionsMap{}
	for _, resource := range registry {
		if resource.Scope != "org" {
			continue
		}
		actions := map[string]bool{}
		for _, action := range resource.Actions {
			actions[action.Action] = CanOrg(userID, orgID, role, Permission{Resource: resource.Resource, Action: action.Action})
		}
		result[resource.Resource] = actions
	}
	return result
}
