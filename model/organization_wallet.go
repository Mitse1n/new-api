package model

import (
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// AdjustPersonalOrganizationQuota keeps rewards and affiliate transfers tied
// to the personal organization, independently of the selected team. The user
// balance remains a compatibility projection, never a second funding source.
func AdjustPersonalOrganizationQuota(orgID, userID int, delta int64) error {
	if delta > int64(common.MaxWalletQuota) || delta < -int64(common.MaxWalletQuota) {
		return ErrWalletQuotaLimitExceeded
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		var org Organization
		if err := lockForUpdate(tx).Where("id = ? AND personal_user_id = ?", orgID, userID).First(&org).Error; err != nil {
			return err
		}
		if delta > 0 && org.Quota > int64(common.MaxWalletQuota)-delta || delta < 0 && org.Quota < -int64(common.MaxWalletQuota)-delta {
			return ErrWalletQuotaLimitExceeded
		}
		org.Quota += delta
		if err := tx.Model(&org).Update("quota", org.Quota).Error; err != nil {
			return err
		}
		return tx.Model(&User{}).Where("id = ?", userID).Update("quota", org.Quota).Error
	})
	if err != nil {
		return err
	}
	if err := invalidateUserCache(userID); err != nil {
		common.SysError("invalidate personal wallet cache: " + err.Error())
	}
	return nil
}

func syncPersonalOrganizationWalletTx(tx *gorm.DB, orgID int) error {
	var org Organization
	if err := tx.Where("id = ?", orgID).First(&org).Error; err != nil {
		return err
	}
	if org.PersonalUserId == nil {
		return nil
	}
	if err := invalidateUserCache(*org.PersonalUserId); err != nil {
		// Organization billing always reads the locked database wallet. Redis
		// failure must not roll back an otherwise valid debit or refund.
		common.SysError("invalidate personal wallet projection: " + err.Error())
	}
	return tx.Model(&User{}).Where("id = ?", *org.PersonalUserId).Update("quota", org.Quota).Error
}

func UpdatePersonalOrganizationGroupTx(tx *gorm.DB, orgID int, group string) error {
	var org Organization
	if err := lockForUpdate(tx).Where("id = ? AND kind = ?", orgID, OrganizationPersonal).First(&org).Error; err != nil {
		return err
	}
	org.Group = group
	if err := tx.Model(&org).Updates(map[string]interface{}{"group": group, "version": gorm.Expr("version + 1")}).Error; err != nil {
		return err
	}
	return RefreshOrganizationTokensTx(tx, &org)
}
