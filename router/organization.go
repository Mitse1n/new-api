package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
)

func setOrganizationRoutes(api *gin.RouterGroup) {
	platform := api.Group("/platform/organizations", middleware.AdminAuth())
	platform.GET("", middleware.RequirePermission(authz.Permission{Resource: "organization", Action: "read"}), controller.PlatformListOrganizations)
	platform.GET("/:org_id/resources/:resource", middleware.RequirePermission(authz.Permission{Resource: "organization", Action: "read"}), controller.PlatformOrganizationResources)
	platform.PUT("/:org_id/status", middleware.RequirePermission(authz.Permission{Resource: "organization", Action: "write"}), controller.PlatformChangeOrganizationStatus)
	organizations := api.Group("/organizations", middleware.UserAuth())
	organizations.GET("", controller.ListOrganizations)
	organizations.GET("/:org_id/deletion-impact", controller.GetOrganizationDeletionImpact)
	organizations.PUT("/:org_id/status", controller.ChangeOrganizationStatus)
	organizations.POST("", middleware.CriticalRateLimit(), controller.CreateOrganization)
	organizations.GET("/invites", controller.ListIncomingOrganizationInvites)
	organizations.POST("/invites/:invite_id/accept", middleware.CriticalRateLimit(), controller.AcceptOrganizationInvite)
	organizations.POST("/invites/:invite_id/decline", middleware.CriticalRateLimit(), controller.DeclineOrganizationInvite)
	org := api.Group("/org", middleware.UserAuth(), middleware.OrganizationContext())
	org.GET("/context", controller.GetOrganizationContext)
	org.GET("/summary", controller.GetOrganizationSummary)
	org.GET("/orders", middleware.RequireOrgPermission("org.billing", "read"), controller.GetOrganizationOrders)
	org.GET("/settings", middleware.RequireOrgPermission("org.settings", "read"), controller.GetOrganizationSettings)
	org.PUT("/settings", middleware.RequireOrgPermission("org.settings", "write"), controller.UpdateOrganizationSettings)
	org.PUT("/members/:user_id/budget", middleware.RequireOrgPermission("org.member", "write"), controller.SetOrganizationMemberBudget)
	org.POST("/transfer", middleware.RequireOrgPermission("org.lifecycle", "write"), controller.RequestOrganizationTransfer)
	org.POST("/transfer/accept", controller.AcceptOrganizationTransfer)
	org.GET("/members", middleware.RequireOrgPermission("org.member", "read"), controller.GetOrganizationMembers)
	org.PUT("/members/:user_id", middleware.RequireOrgPermission("org.member", "write"), controller.UpdateOrganizationMember)
	org.GET("/invites", middleware.RequireOrgPermission("org.member", "write"), controller.GetOrganizationInvites)
	org.POST("/invites", middleware.RequireOrgPermission("org.member", "write"), middleware.CriticalRateLimit(), controller.InviteOrganizationMember)
	org.POST("/invites/:invite_id/resend", middleware.RequireOrgPermission("org.member", "write"), middleware.CriticalRateLimit(), controller.ResendOrganizationInvite)
	org.DELETE("/invites/:invite_id", middleware.RequireOrgPermission("org.member", "write"), controller.RevokeOrganizationInvite)
	org.GET("/logs", middleware.RequireOrgPermission("org.usage", "read"), controller.GetOrganizationLogs)
	org.GET("/logs/stat", middleware.RequireOrgPermission("org.usage", "read"), controller.GetOrganizationLogStats)
	org.GET("/audit", middleware.RequireOrgPermission("org.usage", "read_all"), controller.GetOrganizationAudit)
}
