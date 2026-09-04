package model

import (
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrOrganizationUnsettled = errors.New("organization has a balance, active subscriptions, pending payments or unfinished requests")

type OrganizationDeletionImpact struct {
	Members       int64 `json:"members"`
	Tokens        int64 `json:"tokens"`
	Logs          int64 `json:"logs"`
	Orders        int64 `json:"orders"`
	Subscriptions int64 `json:"subscriptions"`
	Quota         int64 `json:"quota"`
	Blocked       bool  `json:"blocked"`
}

// A disabled team remains accessible only through this explicit owner lifecycle
// operation. Ordinary organization context continues to fail closed.
func lockOrganizationOwner(tx *gorm.DB, orgID, actorID int) (*Organization, error) {
	var org Organization
	if err := lockForUpdate(tx).Where("id = ? AND owner_id = ? AND kind = ?", orgID, actorID, OrganizationTeam).First(&org).Error; err != nil {
		return nil, ErrOrganizationAccess
	}
	var count int64
	if err := tx.Model(&OrganizationMember{}).Scopes(OrgScope(orgID)).Where("user_id = ? AND role = ? AND status = ?", actorID, OrgRoleOwner, OrganizationActive).Count(&count).Error; err != nil {
		return nil, err
	}
	if count != 1 {
		return nil, ErrOrganizationAccess
	}
	return &org, nil
}

func OrganizationHasUnsettledFunds(tx *gorm.DB, org *Organization) (bool, error) {
	if org.Quota != 0 {
		return true, nil
	}
	queries := []*gorm.DB{
		tx.Model(&UserSubscription{}).Scopes(OrgScope(org.Id)).Where("status = ? AND end_time > ?", "active", common.GetTimestamp()),
		tx.Model(&SubscriptionOrder{}).Scopes(OrgScope(org.Id)).Where("status = ?", common.TopUpStatusPending),
		tx.Model(&TopUp{}).Scopes(OrgScope(org.Id)).Where("status = ?", common.TopUpStatusPending),
		tx.Model(&OrganizationCharge{}).Scopes(OrgScope(org.Id)).Where("status = ?", "reserved"),
		tx.Model(&Midjourney{}).Scopes(OrgScope(org.Id)).Where("status NOT IN ?", []string{"SUCCESS", "FAILURE"}),
		tx.Model(&Task{}).Scopes(OrgScope(org.Id)).Where("status NOT IN ?", []string{"SUCCESS", "FAILURE"}),
	}
	for _, query := range queries {
		var count int64
		if err := query.Count(&count).Error; err != nil {
			return false, err
		}
		if count > 0 {
			return true, nil
		}
	}
	return false, nil
}

func GetOrganizationDeletionImpact(orgID, actorID int) (*OrganizationDeletionImpact, error) {
	impact := &OrganizationDeletionImpact{}
	err := DB.Transaction(func(tx *gorm.DB) error {
		org, err := lockOrganizationOwner(tx, orgID, actorID)
		if err != nil {
			return err
		}
		impact.Quota = org.Quota
		impact.Blocked, err = OrganizationHasUnsettledFunds(tx, org)
		if err != nil {
			return err
		}
		for _, entry := range []struct {
			resource interface{}
			count    *int64
		}{
			{&OrganizationMember{}, &impact.Members}, {&Token{}, &impact.Tokens}, {&SubscriptionOrder{}, &impact.Orders}, {&UserSubscription{}, &impact.Subscriptions},
		} {
			if err := tx.Model(entry.resource).Scopes(OrgScope(orgID)).Count(entry.count).Error; err != nil {
				return err
			}
		}
		return LOG_DB.Model(&Log{}).Scopes(OrgScope(orgID)).Count(&impact.Logs).Error
	})
	return impact, err
}

func ChangeOrganizationStatus(orgID, actorID, status int, confirmSlug string) error {
	if status != OrganizationActive && status != OrganizationDisabled && status != OrganizationDeleting {
		return ErrOrganizationInput
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		org, err := lockOrganizationOwner(tx, orgID, actorID)
		if err != nil {
			return err
		}
		if status == OrganizationDeleting {
			if confirmSlug != org.Slug {
				return ErrOrganizationInput
			}
			blocked, err := OrganizationHasUnsettledFunds(tx, org)
			if err != nil {
				return err
			}
			if blocked {
				return ErrOrganizationUnsettled
			}
		}
		org.Status, org.Version = status, org.Version+1
		if err := tx.Model(org).Updates(map[string]interface{}{"status": status, "version": org.Version}).Error; err != nil {
			return err
		}
		if err := RefreshOrganizationTokensTx(tx, org); err != nil {
			return err
		}
		if err := tx.Create(&OrganizationAudit{OrgId: orgID, ActorId: actorID, Action: "organization.status", ObjectId: fmt.Sprint(status), Result: "success"}).Error; err != nil {
			return err
		}
		if status == OrganizationDeleting {
			return tx.Delete(org).Error
		}
		return nil
	})
}

func RequestOrganizationTransfer(orgID, actorID, targetID int) error {
	if actorID == targetID {
		return ErrOrganizationOwner
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		org, err := lockOrganizationManager(tx, orgID, actorID, true)
		if err != nil {
			return err
		}
		if org.Kind != OrganizationTeam {
			return ErrOrganizationOwner
		}
		var target OrganizationMember
		if err := tx.Scopes(OrgScope(orgID)).Where("user_id = ? AND status = ?", targetID, OrganizationActive).First(&target).Error; err != nil {
			return ErrOrganizationAccess
		}
		transfer := OrganizationTransfer{OrgId: orgID, OwnerId: actorID, TargetId: targetID, ExpiresAt: time.Now().Add(7 * 24 * time.Hour).Unix()}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "org_id"}}, DoUpdates: clause.AssignmentColumns([]string{"owner_id", "target_id", "expires_at"})}).Create(&transfer).Error; err != nil {
			return err
		}
		return tx.Create(&OrganizationAudit{OrgId: orgID, ActorId: actorID, Action: "ownership.request", ObjectId: fmt.Sprint(targetID), Result: "success"}).Error
	})
}

func AcceptOrganizationTransfer(orgID, actorID int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var org Organization
		if err := lockForUpdate(tx).Where("id = ? AND status = ? AND kind = ?", orgID, OrganizationActive, OrganizationTeam).First(&org).Error; err != nil {
			return ErrOrganizationAccess
		}
		var transfer OrganizationTransfer
		if err := tx.Scopes(OrgScope(orgID)).Where("target_id = ? AND owner_id = ? AND expires_at > ?", actorID, org.OwnerId, common.GetTimestamp()).First(&transfer).Error; err != nil {
			return ErrOrganizationOwner
		}
		var member OrganizationMember
		if err := tx.Scopes(OrgScope(orgID)).Where("user_id = ? AND status = ?", actorID, OrganizationActive).First(&member).Error; err != nil {
			return ErrOrganizationAccess
		}
		if err := tx.Model(&OrganizationMember{}).Scopes(OrgScope(orgID)).Where("user_id = ? AND role = ?", org.OwnerId, OrgRoleOwner).Update("role", OrgRoleAdmin).Error; err != nil {
			return err
		}
		if err := tx.Model(&member).Update("role", OrgRoleOwner).Error; err != nil {
			return err
		}
		if err := tx.Model(&org).Updates(map[string]interface{}{"owner_id": actorID, "version": gorm.Expr("version + 1")}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&transfer).Error; err != nil {
			return err
		}
		return tx.Create(&OrganizationAudit{OrgId: orgID, ActorId: actorID, Action: "ownership.accept", ObjectId: fmt.Sprint(actorID), Result: "success"}).Error
	})
}

// CleanupDeletedOrganizations is restartable: a tombstone is retained until both
// databases and domain policies have been cleaned. Every deletion stays scoped.
func CleanupDeletedOrganizations() error {
	var orgs []Organization
	if err := DB.Unscoped().Where("status = ? AND deleted_at IS NOT NULL", OrganizationDeleting).Limit(10).Find(&orgs).Error; err != nil {
		return err
	}
	for _, org := range orgs {
		if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
			if err := LOG_DB.Exec("ALTER TABLE logs DELETE WHERE org_id = ? SETTINGS mutations_sync = 2", org.Id).Error; err != nil {
				return err
			}
		} else if err := LOG_DB.Unscoped().Scopes(OrgScope(org.Id)).Delete(&Log{}).Error; err != nil {
			return err
		}
		if err := DB.Transaction(func(tx *gorm.DB) error {
			for _, resource := range []interface{}{&Token{}, &OrganizationMember{}, &OrganizationInvite{}, &OrganizationTransfer{}, &OrganizationCharge{}, &OrganizationNotification{}, &OrganizationAudit{}, &UserSubscription{}, &SubscriptionOrder{}, &TopUp{}, &Task{}, &Midjourney{}, &QuotaData{}, &Redemption{}} {
				if err := tx.Unscoped().Scopes(OrgScope(org.Id)).Delete(resource).Error; err != nil {
					return err
				}
			}
			if err := tx.Where("v1 = ?", fmt.Sprintf("org:%d", org.Id)).Delete(&CasbinRule{}).Error; err != nil {
				return err
			}
			return tx.Unscoped().Where("id = ? AND status = ?", org.Id, OrganizationDeleting).Delete(&Organization{}).Error
		}); err != nil {
			return err
		}
	}
	return nil
}
