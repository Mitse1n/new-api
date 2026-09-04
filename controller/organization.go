package controller

import (
	"errors"
	"gorm.io/gorm"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
)

func organizationError(c *gin.Context, err error) {
	status, code, message := http.StatusInternalServerError, "ORG_INTERNAL", "Request failed"
	switch {
	case errors.Is(err, model.ErrOrganizationAccess):
		status, code, message = http.StatusForbidden, "ORG_FORBIDDEN", "Organization access unavailable"
	case errors.Is(err, model.ErrOrganizationSlug):
		status, code, message = http.StatusBadRequest, "ORG_SLUG", "This organization slug is already in use."
	case errors.Is(err, model.ErrOrganizationMemberExists):
		status, code, message = http.StatusBadRequest, "ORG_MEMBER_EXISTS", "This user is already a member of this organization."
	case errors.Is(err, model.ErrOrganizationInvitePending):
		status, code, message = http.StatusBadRequest, "ORG_INVITE_PENDING", "An invitation is already pending. Resend it from the invitation list."
	case errors.Is(err, model.ErrOrganizationInput):
		status, code, message = http.StatusBadRequest, "ORG_INVALID", "Invalid organization details"
	case errors.Is(err, model.ErrOrganizationInviteUser):
		status, code, message = http.StatusBadRequest, "ORG_INVITE_USER", "No active account matches this username."
	case errors.Is(err, model.ErrOrganizationInviteLegacy):
		status, code, message = http.StatusBadRequest, "ORG_INVITE_LEGACY", "This email invitation is no longer supported. Ask an organization administrator to invite your username."
	case errors.Is(err, model.ErrOrganizationInvite):
		status, code, message = http.StatusBadRequest, "ORG_INVITE", "Invitation unavailable or identity does not match"
	case errors.Is(err, model.ErrOrganizationSeats):
		status, code, message = http.StatusBadRequest, "ORG_SEATS", "Organization member limit reached"
	case errors.Is(err, model.ErrOrganizationOwner):
		status, code, message = http.StatusForbidden, "ORG_OWNER", "Ownership operation is not allowed"
	case errors.Is(err, model.ErrOrganizationUnsettled):
		status, code, message = http.StatusBadRequest, "ORG_UNSETTLED", "Settle the balance and active subscriptions before deleting this organization."
	case errors.Is(err, model.ErrOrganizationQuota):
		status, code, message = http.StatusBadRequest, "ORG_QUOTA", "Organization quota insufficient"
	case errors.Is(err, model.ErrMemberSpendLimit):
		status, code, message = http.StatusBadRequest, "ORG_MEMBER_LIMIT", "Member spending limit reached"
	default:
		common.SysError("organization operation: " + err.Error())
	}
	c.JSON(status, gin.H{"success": false, "code": code, "message": message})
}

func ListOrganizations(c *gin.Context) {
	orgs, err := model.ListUserOrganizations(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, orgs)
}

func CreateOrganization(c *gin.Context) {
	var input struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		organizationError(c, model.ErrOrganizationInput)
		return
	}
	org, err := model.CreateTeamOrganization(c.GetInt("id"), input.Name, input.Slug)
	if err != nil {
		organizationError(c, err)
		return
	}
	common.ApiSuccess(c, org)
}

func GetOrganizationContext(c *gin.Context) {
	org, member, err := model.GetOrganizationMembership(c.GetInt("org_id"), c.GetInt("id"))
	if err != nil {
		organizationError(c, err)
		return
	}
	settings, err := org.EffectiveSettings()
	if err != nil {
		organizationError(c, err)
		return
	}
	if member.Role == model.OrgRoleMember {
		org.Settings = ""
		org.UsedQuota = 0
		org.Quota = 0
	}
	var transfer model.OrganizationTransfer
	result := model.DB.Scopes(model.OrgScope(org.Id)).Where("target_id = ? AND owner_id = ? AND expires_at > ?", member.UserId, org.OwnerId, common.GetTimestamp()).First(&transfer)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		common.ApiError(c, result.Error)
		return
	}

	common.ApiSuccess(c, gin.H{"logo": settings.Logo, "pending_transfer": transfer.Id > 0, "organization": org, "membership": member, "capabilities": gin.H{
		"platform": authz.Capabilities(c.GetInt("id"), c.GetInt("role")),
		"org":      authz.OrganizationCapabilities(c.GetInt("id"), org.Id, member.Role),
	}})
}

func GetOrganizationMembers(c *gin.Context) {
	var members []struct {
		model.OrganizationMember
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
	}
	query := model.DB.Model(&model.OrganizationMember{}).Select("organization_members.*, users.username, users.display_name, users.email").Joins("JOIN users ON users.id = organization_members.user_id").Scopes(model.OrgScope(c.GetInt("org_id")))
	if c.GetString("org_role") == model.OrgRoleMember {
		query = query.Where("organization_members.user_id = ?", c.GetInt("id"))
	}
	if err := query.Order("organization_members.id").Scan(&members).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, members)
}

func UpdateOrganizationMember(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil {
		organizationError(c, model.ErrOrganizationInput)
		return
	}
	var input struct {
		Role       string `json:"role"`
		Status     int    `json:"status"`
		SpendLimit int64  `json:"spend_limit"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		organizationError(c, model.ErrOrganizationInput)
		return
	}
	if err := model.UpdateOrganizationMember(c.GetInt("org_id"), c.GetInt("id"), userID, input.Role, input.Status, input.SpendLimit); err != nil {
		organizationError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func GetOrganizationInvites(c *gin.Context) {
	var invites []model.OrganizationInvite
	if err := model.DB.Scopes(model.OrgScope(c.GetInt("org_id"))).Order("id desc").Find(&invites).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	for i := range invites {
		if invites[i].Status == "pending" && invites[i].ExpiresAt <= common.GetTimestamp() {
			invites[i].Status = "expired"
		}
	}
	common.ApiSuccess(c, invites)
}

func InviteOrganizationMember(c *gin.Context) {
	var input struct {
		Username string `json:"username"`
		Role     string `json:"role"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		organizationError(c, model.ErrOrganizationInput)
		return
	}
	invite, err := model.CreateOrganizationInvite(c.GetInt("org_id"), c.GetInt("id"), input.Username, input.Role)
	if err != nil {
		organizationError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	common.ApiSuccess(c, invite)
}

func AcceptOrganizationInvite(c *gin.Context) {
	inviteID, err := strconv.Atoi(c.Param("invite_id"))
	if err != nil {
		organizationError(c, model.ErrOrganizationInvite)
		return
	}
	orgID, err := model.AcceptOrganizationInvite(c.GetInt("id"), inviteID)
	if err != nil {
		organizationError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"org_id": orgID})
}

func RevokeOrganizationInvite(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("invite_id"))
	if err != nil {
		organizationError(c, model.ErrOrganizationInput)
		return
	}
	if err := model.RevokeOrganizationInvite(c.GetInt("org_id"), c.GetInt("id"), id); err != nil {
		organizationError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func GetOrganizationAudit(c *gin.Context) {
	page := common.GetPageQuery(c)
	query := model.DB.Model(&model.OrganizationAudit{}).Scopes(model.OrgScope(c.GetInt("org_id")))
	var total int64
	if err := query.Count(&total).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	var audits []model.OrganizationAudit
	if err := query.Order("id desc").Limit(page.GetPageSize()).Offset(page.GetStartIdx()).Find(&audits).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	page.SetTotal(int(total))
	page.SetItems(audits)
	common.ApiSuccess(c, page)
}

func ResendOrganizationInvite(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("invite_id"))
	if err != nil {
		organizationError(c, model.ErrOrganizationInput)
		return
	}
	invite, err := model.ResendOrganizationInvite(c.GetInt("org_id"), c.GetInt("id"), id)
	if err != nil {
		organizationError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	common.ApiSuccess(c, invite)
}

func ListIncomingOrganizationInvites(c *gin.Context) {
	invites, err := model.ListIncomingOrganizationInvites(c.GetInt("id"))
	if err != nil {
		organizationError(c, err)
		return
	}
	common.ApiSuccess(c, invites)
}
func DeclineOrganizationInvite(c *gin.Context) {
	inviteID, err := strconv.Atoi(c.Param("invite_id"))
	if err != nil {
		organizationError(c, model.ErrOrganizationInvite)
		return
	}
	if err := model.DeclineOrganizationInvite(c.GetInt("id"), inviteID); err != nil {
		organizationError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}
