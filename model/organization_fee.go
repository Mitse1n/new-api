package model

import (
	"crypto/sha256"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// ChargeOrganizationViolationFee has its own receipt because the failed relay
// reservation has already been refunded. The token adjustment commits with the
// fee, and a replay returns false so callers do not duplicate usage logs.
func ChargeOrganizationViolationFee(orgID, userID, tokenID int, requestID string, amount int) (bool, error) {
	if requestID == "" || amount <= 0 || amount > common.MaxQuota {
		return false, ErrOrganizationInput
	}
	hash := sha256.Sum256([]byte("violation:" + requestID))
	feeID := fmt.Sprintf("%x", hash)
	receipt, err := ReserveOrganizationCharge(orgID, userID, tokenID, feeID, int64(amount))
	if err != nil {
		var previous OrganizationCharge
		if queryErr := DB.Scopes(OrgScope(orgID)).Where("request_id = ? AND user_id = ? AND token_id = ? AND status = ? AND quota = ?", feeID, userID, tokenID, "settled", amount).First(&previous).Error; queryErr == nil {
			return false, nil
		}
		return false, err
	}
	charged := false
	err = DB.Transaction(func(tx *gorm.DB) error {
		var org Organization
		if err := lockForUpdate(tx).Where("id = ?", orgID).First(&org).Error; err != nil {
			return err
		}
		var current OrganizationCharge
		if err := tx.Scopes(OrgScope(orgID)).Where("id = ?", receipt.Id).First(&current).Error; err != nil {
			return err
		}
		if current.Status == "settled" {
			return nil
		}
		if err := finalizeOrganizationChargeTx(tx, orgID, feeID, int64(amount), false, false); err != nil {
			return err
		}
		if tokenID > 0 {
			var token Token
			if err := lockForUpdate(tx).Unscoped().Scopes(OrgScope(orgID)).Where("id = ?", tokenID).First(&token).Error; err != nil {
				return err
			}
			if err := invalidateTokenCacheForMutation(token.Key); err != nil {
				return err
			}
			if err := tx.Unscoped().Model(&token).Updates(map[string]interface{}{"remain_quota": gorm.Expr("remain_quota - ?", amount), "used_quota": gorm.Expr("used_quota + ?", amount)}).Error; err != nil {
				return err
			}
		}
		charged = true
		return nil
	})
	if err != nil {
		_ = FinalizeOrganizationCharge(orgID, feeID, 0, true)
	}
	return charged, err
}
