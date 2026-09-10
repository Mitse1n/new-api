package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func GetAccountSummary(c *gin.Context) {
	userID := c.GetInt("id")
	user, err := model.GetUserById(userID, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	quota, err := model.GetUserQuota(userID, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	scope := model.ResourceScope{UserID: userID}
	var subscriptions []model.UserSubscription
	if err := scope.Apply(model.DB).Order("id desc").Find(&subscriptions).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	var keyCount int64
	var usage struct {
		RequestCount int64
		UsedQuota    int64
	}
	if err := scope.Apply(model.DB.Model(&model.Token{})).Count(&keyCount).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	if err := scope.Apply(model.LOG_DB.Model(&model.Log{})).Where("type = ?", model.LogTypeConsume).Select("COUNT(*) AS request_count, COALESCE(SUM(quota), 0) AS used_quota").Scan(&usage).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	available := max(int64(0), int64(quota))
	publicSubscriptions := make([]subscriptionResponse, 0, len(subscriptions))
	for i := range subscriptions {
		sub := &subscriptions[i]
		publicSubscriptions = append(publicSubscriptions, subscriptionResponse{UserSubscription: sub})
		if sub.Status == "active" && sub.EndTime > common.GetTimestamp() && !sub.AllowWalletOverflow {
			available = 0
		}
	}
	for _, sub := range subscriptions {
		if sub.Status == "active" && sub.EndTime > common.GetTimestamp() {
			if sub.AmountTotal == 0 {
				available = int64(common.MaxWalletQuota)
				break
			}
			available += min(max(int64(0), sub.AmountTotal-sub.AmountUsed), int64(common.MaxWalletQuota)-available)
		}
	}
	common.ApiSuccess(c, gin.H{"available_quota": available, "request_count": usage.RequestCount,
		"quota": quota, "used_quota": usage.UsedQuota, "group": user.Group,
		"subscriptions": publicSubscriptions, "key_count": keyCount})
}
