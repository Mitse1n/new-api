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

// OrganizationContext follows UserAuth. Never infer authorization from a
// platform role or from a previously saved browser selection.
func OrganizationContext() gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID, err := strconv.Atoi(c.GetHeader("X-Org-Id"))
		if c.GetHeader("X-Org-Id") == "" {
			personal, personalErr := model.GetPersonalOrganization(c.GetInt("id"))
			err = personalErr
			if personalErr == nil {
				orgID = personal.Id
			}
		}
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

func RequireOrgPermission(resource, action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !authz.CanOrg(c.GetInt("id"), c.GetInt("org_id"), c.GetString("org_role"), authz.Permission{Resource: resource, Action: action}) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "code": "ORG_FORBIDDEN", "message": "You do not have permission for this action."})
			return
		}
		c.Next()
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
