package model

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

func ValidateOrganizationPlan(tx *gorm.DB, org *Organization, plan *SubscriptionPlan) error {
	if org == nil || plan == nil || !plan.Enabled || plan.MaxMembers < 0 {
		return ErrOrganizationInput
	}
	audience := plan.Audience
	if audience == "" {
		audience = "both"
	}
	if audience != "both" && (org.Kind == OrganizationPersonal && audience != "personal" || org.Kind == OrganizationTeam && audience != "org") {
		return errors.New("plan is not available for this organization")
	}
	if plan.MaxMembers > 0 {
		var members, pending int64
		if err := tx.Model(&OrganizationMember{}).Scopes(OrgScope(org.Id)).Where("status = ?", OrganizationActive).Count(&members).Error; err != nil {
			return err
		}
		if err := tx.Model(&OrganizationInvite{}).Scopes(OrgScope(org.Id)).Where("status = ? AND expires_at > ?", "pending", common.GetTimestamp()).Count(&pending).Error; err != nil {
			return err
		}
		if members+pending > int64(plan.MaxMembers) {
			return ErrOrganizationSeats
		}
	}
	if plan.MaxPurchasePerUser > 0 {
		var count, pending int64
		if err := tx.Model(&UserSubscription{}).Scopes(OrgScope(org.Id)).Where("plan_id = ?", plan.Id).Count(&count).Error; err != nil {
			return err
		}
		if err := tx.Model(&SubscriptionOrder{}).Scopes(OrgScope(org.Id)).Where("plan_id = ? AND status = ?", plan.Id, common.TopUpStatusPending).Count(&pending).Error; err != nil {
			return err
		}
		if count+pending >= int64(plan.MaxPurchasePerUser) {
			return errors.New("plan purchase limit reached")
		}
	}
	return nil
}

func CreateOrganizationSubscriptionFromPlanTx(tx *gorm.DB, orgID, actorID int, plan *SubscriptionPlan, source string) (*UserSubscription, error) {
	var org Organization
	if err := lockForUpdate(tx).Where("id = ?", orgID).First(&org).Error; err != nil {
		return nil, err
	}
	// Completion can arrive after suspension or a platform plan edit. Honor the
	// immutable order snapshot; only new checkout requires an enabled plan.
	now := time.Unix(GetDBTimestamp(), 0)
	end, err := calcPlanEndTime(now, plan)
	if err != nil {
		return nil, err
	}
	nextReset := calcNextResetTime(now, plan, end)
	overflow := plan.AllowWalletOverflow == nil || *plan.AllowWalletOverflow
	sub := &UserSubscription{OrgId: orgID, UserId: actorID, PlanId: plan.Id, AmountTotal: plan.TotalAmount,
		StartTime: now.Unix(), EndTime: end, Status: "active", Source: source,
		LastResetTime: now.Unix(), NextResetTime: nextReset, UpgradeGroup: strings.TrimSpace(plan.UpgradeGroup),
		PrevUserGroup: org.Group, DowngradeGroup: strings.TrimSpace(plan.DowngradeGroup), AllowWalletOverflow: overflow}
	snapshot, err := common.Marshal(plan)
	if err != nil {
		return nil, err
	}
	sub.PlanSnapshot = string(snapshot)
	if err := tx.Create(sub).Error; err != nil {
		return nil, err
	}
	if sub.UpgradeGroup != "" && org.Group != sub.UpgradeGroup {
		org.Group = sub.UpgradeGroup
		if err := tx.Model(&org).Updates(map[string]interface{}{"group": org.Group, "version": gorm.Expr("version + 1")}).Error; err != nil {
			return nil, err
		}
		if org.PersonalUserId != nil {
			if err := tx.Model(&User{}).Where("id = ?", *org.PersonalUserId).Update("group", org.Group).Error; err != nil {
				return nil, err
			}
		}
		if err := RefreshOrganizationTokensTx(tx, &org); err != nil {
			return nil, err
		}
	}
	return sub, nil
}

func downgradeOrganizationSubscriptionTx(tx *gorm.DB, sub *UserSubscription, now int64) (string, error) {
	var org Organization
	if err := lockForUpdate(tx).Where("id = ?", sub.OrgId).First(&org).Error; err != nil {
		return "", err
	}
	var active UserSubscription
	if err := tx.Scopes(OrgScope(sub.OrgId)).Where("status = ? AND end_time > ? AND id <> ? AND upgrade_group <> ?", "active", now, sub.Id, "").Order("start_time DESC, id DESC").Limit(1).Find(&active).Error; err != nil {
		return "", err
	}
	group := active.UpgradeGroup
	if active.Id == 0 {
		group = sub.DowngradeGroup
		if group == "" {
			if org.Group != sub.UpgradeGroup {
				return "", nil
			}
			group = sub.PrevUserGroup
			// Walk purchased snapshots backwards when the previous tier also
			// expired, so stacked plans cannot leave an expired tier active.
			var previous []UserSubscription
			if err := tx.Scopes(OrgScope(sub.OrgId)).Where("id < ? AND upgrade_group <> ?", sub.Id, "").Order("id DESC").Find(&previous).Error; err != nil {
				return "", err
			}
			for _, prior := range previous {
				if prior.UpgradeGroup != group || prior.Status == "active" && prior.EndTime > now {
					continue
				}
				if prior.DowngradeGroup != "" {
					group = prior.DowngradeGroup
				} else {
					group = prior.PrevUserGroup
				}
			}
		}
	}
	if group == "" || group == org.Group {
		return "", nil
	}
	org.Group = group
	if err := tx.Model(&org).Updates(map[string]interface{}{"group": group, "version": gorm.Expr("version + 1")}).Error; err != nil {
		return "", err
	}
	if org.PersonalUserId != nil {
		if err := tx.Model(&User{}).Where("id = ?", *org.PersonalUserId).Update("group", group).Error; err != nil {
			return "", err
		}
	}
	return group, RefreshOrganizationTokensTx(tx, &org)
}

func PurchaseOrganizationSubscriptionWithBalance(orgID, actorID, planID int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		org, err := lockOrganizationManager(tx, orgID, actorID, false)
		if err != nil {
			return err
		}
		plan, err := getSubscriptionPlanByIdTx(tx, planID)
		if err != nil {
			return err
		}
		if err := ValidateOrganizationPlan(tx, org, plan); err != nil {
			return err
		}
		if plan.AllowBalancePay != nil && !*plan.AllowBalancePay {
			return errors.New("balance payment is unavailable")
		}
		quota, err := calcSubscriptionBalanceQuota(plan.PriceAmount)
		if err != nil {
			return err
		}
		if org.Quota < int64(quota) {
			return ErrOrganizationQuota
		}
		if err := tx.Model(org).Update("quota", gorm.Expr("quota - ?", quota)).Error; err != nil {
			return err
		}
		if _, err := CreateOrganizationSubscriptionFromPlanTx(tx, orgID, actorID, plan, PaymentMethodBalance); err != nil {
			return err
		}
		snapshot, err := common.Marshal(plan)
		if err != nil {
			return err
		}
		now := common.GetTimestamp()
		order := SubscriptionOrder{OrgId: orgID, UserId: actorID, PlanId: planID, Money: plan.PriceAmount, PlanSnapshot: string(snapshot),
			TradeNo: fmt.Sprintf("SUBORG%d-%s-%d", orgID, common.GetRandomString(12), time.Now().UnixNano()), PaymentMethod: PaymentMethodBalance,
			PaymentProvider: PaymentProviderBalance, Status: common.TopUpStatusSuccess, CreateTime: now, CompleteTime: now}
		if err := tx.Create(&order).Error; err != nil {
			return err
		}
		if err := tx.Create(&OrganizationAudit{OrgId: orgID, ActorId: actorID, Action: "subscription.paid", ObjectId: fmt.Sprint(order.Id), Result: "success"}).Error; err != nil {
			return err
		}
		return syncPersonalOrganizationWalletTx(tx, orgID)
	})
}

func creditOrganizationTopUp(tx *gorm.DB, orgID int, quota int) error {
	max, err := topUpQuotaMaxCurrent(quota)
	if err != nil {
		return err
	}
	result := tx.Model(&Organization{}).Where("id = ? AND quota <= ?", orgID, max).Update("quota", gorm.Expr("quota + ?", quota))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrTopUpQuotaLimitExceeded
	}
	return syncPersonalOrganizationWalletTx(tx, orgID)
}

// GetOrganizationSubscriptionPlan preserves purchased terms after edits or removal
// of the catalog plan. Legacy subscriptions retain their existing plan semantics.
func GetOrganizationSubscriptionPlan(tx *gorm.DB, sub *UserSubscription) (*SubscriptionPlan, error) {
	if sub.PlanSnapshot == "" {
		return getSubscriptionPlanByIdTx(tx, sub.PlanId)
	}
	plan := &SubscriptionPlan{}
	if err := common.UnmarshalJsonStr(sub.PlanSnapshot, plan); err != nil {
		return nil, err
	}
	return plan, nil
}
