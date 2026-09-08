package middleware

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
)

// TeamOrganizationContext follows UserAuth and resolves the team named by
// X-Org-Id. Without that header the request stays in the caller's own scope and
// no organization is set, so handlers must treat a zero org_id as "the caller"
// rather than as an absent context. Never infer authorization from a platform
// role or from a previously saved browser selection.
func TeamOrganizationContext() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("X-Org-Id")
		if header == "" {
			c.Next()
			return
		}
		orgID, err := strconv.Atoi(header)
		if err != nil || orgID <= 0 {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "code": "ORG_UNAVAILABLE", "message": "Organization unavailable."})
			return
		}
		org, member, err := model.GetOrganizationMembership(orgID, c.GetInt("id"))
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, model.ErrOrganizationAccess) {
				status = http.StatusForbidden
			}
			c.AbortWithStatusJSON(status, gin.H{"success": false, "code": "ORG_UNAVAILABLE", "message": "Organization unavailable."})
			return
		}
		common.SetContextKey(c, constant.ContextKeyOrgId, org.Id)
		common.SetContextKey(c, constant.ContextKeyOrgRole, member.Role)
		common.SetContextKey(c, constant.ContextKeyOrganization, org)
		common.SetContextKey(c, constant.ContextKeyUserGroup, org.Group)
		c.Set("group", org.Group)
		settings, err := org.EffectiveSettings()
		if err != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		if len(settings.AllowedModels) > 0 {
			limits := make(map[string]bool, len(settings.AllowedModels))
			for _, name := range settings.AllowedModels {
				limits[name] = true
			}
			c.Set("token_model_limit_enabled", true)
			c.Set("token_model_limit", limits)
		}
		c.Next()
		if c.Writer.Status() >= http.StatusBadRequest && c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			model.RecordOrganizationRequestFailure(org.Id, c.GetInt("id"), c.Writer.Status(), c.Request.Method+" "+c.FullPath())
		}
	}
}

// RequireTeamOrganization guards the organization endpoints, which exist only
// for teams. It rejects a request that carried no X-Org-Id at all.
func RequireTeamOrganization() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetInt("org_id") <= 0 {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "code": "ORG_UNAVAILABLE", "message": "Organization unavailable."})
			return
		}
		c.Next()
	}
}

// RequireOrgPermission gates an action within a team. Shared endpoints such as
// top-up and subscription purchase serve both a team and a plain account, and
// an account acting for itself needs no organization role, so a request with no
// organization passes through. Team-only endpoints sit behind
// RequireTeamOrganization, which already rejects a missing organization.
func RequireOrgPermission(resource, action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID := c.GetInt("org_id")
		if orgID <= 0 {
			c.Next()
			return
		}
		if !authz.CanOrg(c.GetInt("id"), orgID, c.GetString("org_role"), authz.Permission{Resource: resource, Action: action}) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "code": "ORG_FORBIDDEN", "message": "You do not have permission for this action."})
			return
		}
		c.Next()
	}
}
