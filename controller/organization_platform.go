package controller

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// These endpoints are deliberately separate from ordinary organization context.
// Platform authorization never turns org_id=0 into a wildcard in OrgScope.
func PlatformListOrganizations(c *gin.Context) {
	page := common.GetPageQuery(c)
	query := model.DB.Model(&model.Organization{})
	if keyword := strings.TrimSpace(c.Query("keyword")); keyword != "" {
		query = query.Where("name LIKE ? OR slug LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	var organizations []model.Organization
	if err := query.Select("id", "name", "slug", "kind", "status", "owner_id", "quota", "used_quota", "group").Order("id desc").Offset(page.GetStartIdx()).Limit(page.GetPageSize()).Find(&organizations).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	page.SetTotal(int(total))
	page.SetItems(organizations)
	common.ApiSuccess(c, page)
}

func PlatformOrganizationResources(c *gin.Context) {
	orgID, err := strconv.Atoi(c.Param("org_id"))
	if err != nil || orgID <= 0 {
		organizationError(c, model.ErrOrganizationInput)
		return
	}
	page := common.GetPageQuery(c)
	var resource interface{}
	database := model.DB
	switch c.Param("resource") {
	case "members":
		resource = &[]model.OrganizationMember{}
	case "tokens":
		resource = &[]model.Token{}
	case "orders":
		resource = &[]model.SubscriptionOrder{}
	case "subscriptions":
		resource = &[]model.UserSubscription{}
	case "audit":
		resource = &[]model.OrganizationAudit{}
	case "logs":
		resource = &[]model.Log{}
		database = model.LOG_DB
	default:
		organizationError(c, model.ErrOrganizationInput)
		return
	}
	query := database.Model(resource).Scopes(model.OrgScope(orgID))
	var total int64
	if err := query.Count(&total).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	if err := query.Order("id desc").Offset(page.GetStartIdx()).Limit(page.GetPageSize()).Find(resource).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	var items interface{} = resource
	if tokens, ok := resource.(*[]model.Token); ok {
		masked := make([]interface{}, 0, len(*tokens))
		for i := range *tokens {
			masked = append(masked, buildMaskedTokenResponse(&(*tokens)[i]))
		}
		items = masked
	}
	if orders, ok := resource.(*[]model.SubscriptionOrder); ok {
		for i := range *orders {
			(*orders)[i].ProviderPayload = ""
		}
	}
	page.SetTotal(int(total))
	page.SetItems(items)
	common.ApiSuccess(c, page)
}

func PlatformChangeOrganizationStatus(c *gin.Context) {
	orgID, err := strconv.Atoi(c.Param("org_id"))
	var input struct {
		Status int    `json:"status"`
		Reason string `json:"reason"`
	}
	if err != nil || orgID <= 0 || c.ShouldBindJSON(&input) != nil || strings.TrimSpace(input.Reason) == "" || len(input.Reason) > 256 || (input.Status != model.OrganizationActive && input.Status != model.OrganizationDisabled) {
		organizationError(c, model.ErrOrganizationInput)
		return
	}
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		return model.PlatformChangeOrganizationStatusTx(tx, orgID, c.GetInt("id"), input.Status, input.Reason)
	})
	if err != nil {
		organizationError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}
