package model

import (
	"fmt"
	"gorm.io/gorm"
)

func PlatformChangeOrganizationStatusTx(tx *gorm.DB, orgID, actorID, status int, reason string) error {
	var org Organization
	if err := lockForUpdate(tx).Where("id = ?", orgID).First(&org).Error; err != nil {
		return ErrOrganizationAccess
	}
	if status != OrganizationActive && status != OrganizationDisabled {
		return ErrOrganizationInput
	}
	if status == OrganizationDisabled {
		status = OrganizationSuspended
	}
	org.Status = status
	if err := tx.Model(&org).Update("status", status).Error; err != nil {
		return err
	}
	return tx.Create(&OrganizationAudit{OrgId: orgID, ActorId: actorID, Action: "platform.status", ObjectId: fmt.Sprint(status), Result: "success", Reason: reason}).Error
}
