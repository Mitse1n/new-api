package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
)

func GetOrganizationSummary(c *gin.Context) {
	org, member, err := model.GetOrganizationMembership(c.GetInt("org_id"), c.GetInt("id"))
	if err != nil {
		organizationError(c, err)
		return
	}
	usage, err := model.GetOrganizationBudgetUsage(org.Id, org.BudgetPeriodStart)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !authz.CanOrg(member.UserId, org.Id, member.Role, authz.Permission{Resource: "org.usage", Action: "read_all"}) {
		var ownUsed int64
		if err := model.DB.Model(&model.OrganizationCharge{}).Scopes(model.OrgScope(org.Id)).Where("user_id = ? AND status = ?", member.UserId, "settled").Select("COALESCE(SUM(quota), 0)").Scan(&ownUsed).Error; err != nil {
			common.ApiError(c, err)
			return
		}
		org.UsedQuota = ownUsed
		own := make([]model.OrganizationBudgetUsage, 0, 1)
		for _, row := range usage {
			if row.UserId == member.UserId {
				own = append(own, row)
			}
		}
		usage = own
	}
	var subscriptions []model.UserSubscription
	if err := model.DB.Scopes(model.OrgScope(org.Id)).Order("id desc").Find(&subscriptions).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	var memberCount, keyCount, requestCount int64
	if err := organizationUsageScope(c).Apply(model.LOG_DB.Model(&model.Log{})).Where("type = ?", model.LogTypeConsume).Count(&requestCount).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.DB.Model(&model.OrganizationMember{}).Scopes(model.OrgScope(org.Id)).Where("status = ?", model.OrganizationActive).Count(&memberCount).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	if err := tokenOrganizationScope(c).Apply(model.DB.Model(&model.Token{})).Count(&keyCount).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	settings, err := org.EffectiveSettings()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	available := max(int64(0), org.Quota)
	for _, sub := range subscriptions {
		if sub.Status == "active" && sub.EndTime > common.GetTimestamp() && !sub.AllowWalletOverflow {
			available = 0
			break
		}
	}
	for _, sub := range subscriptions {
		if sub.Status != "active" || sub.EndTime <= common.GetTimestamp() {
			continue
		}
		if sub.AmountTotal == 0 {
			available = int64(common.MaxWalletQuota)
			break
		}
		available += min(max(int64(0), sub.AmountTotal-sub.AmountUsed), int64(common.MaxWalletQuota)-available)
	}
	if member.SpendLimit > 0 {
		remaining := member.SpendLimit
		for _, row := range usage {
			if row.UserId == member.UserId {
				remaining -= min(remaining, row.Used)
				remaining -= min(remaining, row.Reserved)
			}
		}
		available = min(available, max(int64(0), remaining))
	}
	if !authz.CanOrg(member.UserId, org.Id, member.Role, authz.Permission{Resource: "org.billing", Action: "read"}) {
		org.Quota, settings.BudgetLimit = available, 0
		subscriptions = nil
		memberCount = 1
	}
	common.ApiSuccess(c, gin.H{"available_quota": available, "request_count": requestCount, "quota": org.Quota, "used_quota": org.UsedQuota, "group": org.Group, "period_start": org.BudgetPeriodStart, "period_end": org.BudgetPeriodEnd, "usage": usage, "subscriptions": subscriptions, "member_count": memberCount, "key_count": keyCount, "spend_limit": member.SpendLimit, "budget_limit": settings.BudgetLimit, "alert_percent": settings.AlertPercent})
}

func GetOrganizationSettings(c *gin.Context) {
	org, _, err := model.GetOrganizationMembership(c.GetInt("org_id"), c.GetInt("id"))
	if err != nil {
		organizationError(c, err)
		return
	}
	settings, err := org.EffectiveSettings()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var transfers []model.OrganizationTransfer
	if err := model.DB.Scopes(model.OrgScope(org.Id)).Where("expires_at > ?", common.GetTimestamp()).Find(&transfers).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"name": org.Name, "settings": settings, "available_models": model.GetGroupEnabledModels(org.Group), "transfers": transfers})
}

func UpdateOrganizationSettings(c *gin.Context) {
	var input struct {
		Name     string                     `json:"name"`
		Settings model.OrganizationSettings `json:"settings"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		organizationError(c, model.ErrOrganizationInput)
		return
	}
	if err := model.UpdateOrganizationSettings(c.GetInt("org_id"), c.GetInt("id"), input.Name, input.Settings); err != nil {
		organizationError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func SetOrganizationMemberBudget(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil {
		organizationError(c, model.ErrOrganizationInput)
		return
	}
	var input struct {
		SpendLimit int64 `json:"spend_limit"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		organizationError(c, model.ErrOrganizationInput)
		return
	}
	if err := model.SetOrganizationMemberBudget(c.GetInt("org_id"), c.GetInt("id"), userID, input.SpendLimit); err != nil {
		organizationError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func GetOrganizationDeletionImpact(c *gin.Context) {
	orgID, err := strconv.Atoi(c.Param("org_id"))
	if err != nil {
		organizationError(c, model.ErrOrganizationInput)
		return
	}
	impact, err := model.GetOrganizationDeletionImpact(orgID, c.GetInt("id"))
	if err != nil {
		organizationError(c, err)
		return
	}
	common.ApiSuccess(c, impact)
}

func ChangeOrganizationStatus(c *gin.Context) {
	orgID, err := strconv.Atoi(c.Param("org_id"))
	if err != nil {
		organizationError(c, model.ErrOrganizationInput)
		return
	}
	var input struct {
		Status      int    `json:"status"`
		ConfirmSlug string `json:"confirm_slug"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		organizationError(c, model.ErrOrganizationInput)
		return
	}
	if err := model.ChangeOrganizationStatus(orgID, c.GetInt("id"), input.Status, input.ConfirmSlug); err != nil {
		organizationError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func RequestOrganizationTransfer(c *gin.Context) {
	var input struct {
		TargetId int `json:"target_id"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		organizationError(c, model.ErrOrganizationInput)
		return
	}
	if err := model.RequestOrganizationTransfer(c.GetInt("org_id"), c.GetInt("id"), input.TargetId); err != nil {
		organizationError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func AcceptOrganizationTransfer(c *gin.Context) {
	if err := model.AcceptOrganizationTransfer(c.GetInt("org_id"), c.GetInt("id")); err != nil {
		organizationError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func GetOrganizationOrders(c *gin.Context) {
	page := common.GetPageQuery(c)
	query := model.DB.Model(&model.SubscriptionOrder{}).Scopes(model.OrgScope(c.GetInt("org_id")))
	var total int64
	if err := query.Count(&total).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	orders := []model.SubscriptionOrder{}
	if err := query.Order("id desc").Offset(page.GetStartIdx()).Limit(page.GetPageSize()).Find(&orders).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	// Render the title purchased at checkout, even if the platform renames or
	// removes the plan later. Provider contact data never enters this response.
	type organizationOrder struct {
		model.SubscriptionOrder
		PlanTitle string `json:"plan_title"`
	}
	items := make([]organizationOrder, 0, len(orders))
	for _, order := range orders {
		order.ProviderPayload = ""
		var plan model.SubscriptionPlan
		if order.PlanSnapshot != "" {
			if err := common.UnmarshalJsonStr(order.PlanSnapshot, &plan); err != nil {
				common.ApiError(c, err)
				return
			}
		}
		items = append(items, organizationOrder{SubscriptionOrder: order, PlanTitle: plan.Title})
	}
	page.SetTotal(int(total))
	page.SetItems(items)
	common.ApiSuccess(c, page)
}
