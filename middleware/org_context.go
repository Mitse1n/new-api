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

// OrganizationContext resolves an explicitly selected team after UserAuth.
// Without X-Org-Id, resources and billing belong directly to the user.
func OrganizationContext() gin.HandlerFunc {
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

// Explicit organization endpoints require a validated team context.
func RequireOrganization() gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, _ := c.Get("organization")
		org, ok := raw.(*model.Organization)
		if !ok || org == nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "code": "ORG_UNAVAILABLE", "message": "Organization unavailable."})
			return
		}
		c.Next()
	}
}

func RequireOrgPermission(resource, action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !authz.CanOrg(c.GetInt("id"), c.GetInt("org_id"), c.GetString("org_role"), authz.Permission{Resource: resource, Action: action}) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "code": "ORG_FORBIDDEN", "message": "You do not have permission for this action."})
			return
		}
		c.Next()
	}
}

// RequireSelectedOrgPermission checks team permissions on shared account/team
// routes. UserAuth already authorizes access to the user's own resources.
func RequireSelectedOrgPermission(resource, action string) gin.HandlerFunc {
	check := RequireOrgPermission(resource, action)
	return func(c *gin.Context) {
		if c.GetInt("org_id") == 0 && c.GetInt("id") > 0 {
			c.Next()
			return
		}
		check(c)
	}
}

// OptionalOrganizationContext preserves public price browsing while using the
// selected organization's purchased group for authenticated visitors.
func OptionalOrganizationContext() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetInt("id") <= 0 {
			c.Next()
			return
		}
		OrganizationContext()(c)
	}
}
