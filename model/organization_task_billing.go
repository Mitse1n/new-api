package model

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// SettleOrganizationTaskQuota updates the original funding source, token and
// durable task amount together. Repeating a poll after a crash cannot refund
// either the organization wallet or token a second time.
func SettleOrganizationTaskQuota(task *Task, actual int) (int, error) {
	if task.OrgId <= 0 || actual < 0 || actual > common.MaxQuota {
		return 0, ErrOrganizationInput
	}
	delta := 0
	err := DB.Transaction(func(tx *gorm.DB) error {
		var org Organization
		if err := lockForUpdate(tx).Where("id = ?", task.OrgId).First(&org).Error; err != nil {
			return err
		}
		var current Task
		if err := lockForUpdate(tx).Scopes(OrgScope(task.OrgId)).Where("id = ?", task.ID).First(&current).Error; err != nil {
			return err
		}
		delta = actual - current.Quota
		if delta == 0 {
			return nil
		}
		requestID := current.PrivateData.BillingRequestId
		if requestID == "" {
			requestID = fmt.Sprintf("migrated-task:%d", current.ID)
		}
		if err := adjustOrganizationAssetQuotaTx(tx, &org, current.UserId, current.PrivateData.TokenId, requestID, current.PrivateData.SubscriptionId, current.Quota, actual); err != nil {
			return err
		}
		return tx.Model(&current).Update("quota", actual).Error
	})
	if err == nil {
		task.Quota = actual
	}
	return delta, err
}

func RefundOrganizationMidjourneyQuota(task *Midjourney) (int, error) {
	if task.OrgId <= 0 {
		return 0, ErrOrganizationInput
	}
	refunded := 0
	err := DB.Transaction(func(tx *gorm.DB) error {
		var org Organization
		if err := lockForUpdate(tx).Where("id = ?", task.OrgId).First(&org).Error; err != nil {
			return err
		}
		var current Midjourney
		if err := lockForUpdate(tx).Scopes(OrgScope(task.OrgId)).Where("id = ?", task.Id).First(&current).Error; err != nil {
			return err
		}
		refunded = current.Quota
		if refunded == 0 {
			return nil
		}
		requestID := current.BillingRequestId
		if requestID == "" {
			requestID = fmt.Sprintf("migrated-mj:%d", current.Id)
		}
		if err := adjustOrganizationAssetQuotaTx(tx, &org, current.UserId, current.TokenId, requestID, current.SubscriptionId, current.Quota, 0); err != nil {
			return err
		}
		return tx.Model(&current).Update("quota", 0).Error
	})
	if err == nil {
		task.Quota = 0
	}
	return refunded, err
}

func adjustOrganizationAssetQuotaTx(tx *gorm.DB, org *Organization, userID, tokenID int, requestID string, subscriptionID, previous, actual int) error {
	var receipt OrganizationCharge
	result := tx.Scopes(OrgScope(org.Id)).Where("request_id = ?", requestID).Find(&receipt)
	if result.Error != nil {
		return result.Error
	}
	if receipt.Id == 0 {
		// Only migrated tasks lack a request receipt: their initial charge was
		// already included in the personal wallet copied during migration.
		receipt = OrganizationCharge{RequestId: requestID, OrgId: org.Id, UserId: userID, TokenId: tokenID, Quota: int64(previous), Status: "settled", SubscriptionId: subscriptionID, PeriodStart: org.BudgetPeriodStart}
		if subscriptionID > 0 {
			var sub UserSubscription
			if err := tx.Scopes(OrgScope(org.Id)).Where("id = ?", subscriptionID).First(&sub).Error; err != nil {
				return err
			}
			receipt.SubscriptionPeriod = sub.LastResetTime
		}
		if err := tx.Create(&receipt).Error; err != nil {
			return err
		}
	}
	if err := finalizeOrganizationChargeTx(tx, org.Id, requestID, int64(actual), actual == 0, true); err != nil {
		return err
	}
	delta := actual - previous
	if receipt.TokenQuotaManaged || tokenID <= 0 || delta == 0 {
		return nil
	}
	var token Token
	if err := tx.Unscoped().Scopes(OrgScope(org.Id)).Where("id = ?", tokenID).First(&token).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if err := invalidateTokenCacheForMutation(token.Key); err != nil {
		return err
	}
	return tx.Unscoped().Model(&token).Updates(map[string]interface{}{"remain_quota": gorm.Expr("remain_quota - ?", delta), "used_quota": gorm.Expr("used_quota + ?", delta)}).Error
}
