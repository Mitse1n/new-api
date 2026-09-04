package model

import (
	"errors"
	"math"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

var (
	ErrOrganizationTokenQuota = errors.New("token quota insufficient")
	ErrOrganizationQuota      = errors.New("organization quota insufficient")
	ErrMemberSpendLimit       = errors.New("member spending limit reached")
)

// OrganizationCharge is the durable request receipt and reservation. It is not
// a member wallet: money is debited exclusively from the organization wallet
// or its subscription. Receipts make retries and asynchronous refunds safe.
type OrganizationCharge struct {
	Id                 int    `json:"id"`
	RequestId          string `json:"request_id" gorm:"type:varchar(64);uniqueIndex;not null"`
	OrgId              int    `json:"org_id" gorm:"index:idx_org_charge_period,priority:1;not null"`
	UserId             int    `json:"user_id" gorm:"index:idx_org_charge_period,priority:2;not null"`
	TokenId            int    `json:"token_id"`
	TokenQuotaManaged  bool   `json:"-"`
	PeriodStart        int64  `json:"period_start" gorm:"type:bigint;index:idx_org_charge_period,priority:3"`
	SubscriptionId     int    `json:"subscription_id"`
	SubscriptionPeriod int64  `json:"-" gorm:"type:bigint"`
	Quota              int64  `json:"quota" gorm:"type:bigint;not null"`
	Status             string `json:"status" gorm:"type:varchar(16);not null"`
	CreatedAt          int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt          int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

type OrganizationBudgetUsage struct {
	UserId   int   `json:"user_id"`
	Used     int64 `json:"used"`
	Reserved int64 `json:"reserved"`
}

func GetOrganizationBudgetUsage(orgID int, periodStart int64) ([]OrganizationBudgetUsage, error) {
	usage := make([]OrganizationBudgetUsage, 0)
	err := DB.Model(&OrganizationCharge{}).Scopes(OrgScope(orgID)).Where("period_start = ?", periodStart).
		Select("user_id, SUM(CASE WHEN status = 'settled' THEN quota ELSE 0 END) AS used, SUM(CASE WHEN status = 'reserved' THEN quota ELSE 0 END) AS reserved").Group("user_id").Scan(&usage).Error
	return usage, err
}

// ReserveOrganizationCharge reserves a target amount, not a delta. Retrying
// the same request cannot double debit. Concurrent reservations are included
// in the member precheck so delayed consume logs cannot permit overspending.
func ReserveOrganizationCharge(orgID, userID, tokenID int, requestID string, amount int64) (*OrganizationCharge, error) {
	return reserveOrganizationCharge(orgID, userID, tokenID, requestID, amount, false)
}

// ReserveOrganizationRequest commits the Key precheck and counters with the
// organization reservation. Redis stores identity, never authoritative quota.
func ReserveOrganizationRequest(orgID, userID, tokenID int, requestID string, amount int64) (*OrganizationCharge, error) {
	return reserveOrganizationCharge(orgID, userID, tokenID, requestID, amount, true)
}

func reserveOrganizationCharge(orgID, userID, tokenID int, requestID string, amount int64, manageToken bool) (*OrganizationCharge, error) {
	if orgID <= 0 || userID <= 0 || requestID == "" || len(requestID) > 64 || amount < 0 || amount > int64(common.MaxQuota) {
		return nil, ErrOrganizationInput
	}
	var receipt OrganizationCharge
	err := DB.Transaction(func(tx *gorm.DB) error {
		var org Organization
		if err := lockForUpdate(tx).Where("id = ? AND status = ?", orgID, OrganizationActive).First(&org).Error; err != nil {
			return ErrOrganizationAccess
		}
		result := tx.Where("request_id = ?", requestID).Limit(1).Find(&receipt)
		if result.Error != nil {
			return result.Error
		}
		if receipt.Id != 0 {
			if receipt.OrgId != orgID || receipt.UserId != userID || receipt.TokenId != tokenID || receipt.TokenQuotaManaged != manageToken {
				return ErrOrganizationAccess
			}
			if receipt.Status != "reserved" {
				return errors.New("request has already been finalized")
			}
			if amount <= receipt.Quota {
				return nil
			}
		}
		var member OrganizationMember
		// A key remains an organization asset after its creator leaves. The
		// historical membership still supplies its budget attribution.
		if err := tx.Scopes(OrgScope(orgID)).Where("user_id = ?", userID).First(&member).Error; err != nil {
			return ErrOrganizationAccess
		}
		now := common.GetTimestamp()
		var subs []UserSubscription
		if err := lockForUpdate(tx).Scopes(OrgScope(orgID)).Where("status = ? AND end_time > ?", "active", now).Order("end_time, id").Find(&subs).Error; err != nil {
			return err
		}
		for i := range subs {
			plan, err := GetOrganizationSubscriptionPlan(tx, &subs[i])
			if err != nil {
				return err
			}
			if err := maybeResetUserSubscriptionWithPlanTx(tx, &subs[i], plan, now); err != nil {
				return err
			}
		}
		if org.BudgetPeriodEnd <= now {
			date := time.Unix(now, 0).UTC()
			start := time.Date(date.Year(), date.Month(), 1, 0, 0, 0, 0, time.UTC)
			org.BudgetPeriodStart, org.BudgetPeriodEnd = start.Unix(), start.AddDate(0, 1, 0).Unix()
			if len(subs) > 0 {
				org.BudgetPeriodStart = subs[0].LastResetTime
				if org.BudgetPeriodStart <= 0 {
					org.BudgetPeriodStart = subs[0].StartTime
				}
				org.BudgetPeriodEnd = subs[0].NextResetTime
				if org.BudgetPeriodEnd <= now {
					org.BudgetPeriodEnd = subs[0].EndTime
				}
			}
			if err := tx.Model(&org).Updates(map[string]interface{}{"budget_period_start": org.BudgetPeriodStart, "budget_period_end": org.BudgetPeriodEnd}).Error; err != nil {
				return err
			}
		}
		period := org.BudgetPeriodStart
		if receipt.Id != 0 {
			period = receipt.PeriodStart
		}
		delta := amount - receipt.Quota
		if manageToken && tokenID > 0 {
			if err := adjustOrganizationTokenQuotaTx(tx, orgID, userID, tokenID, delta, true); err != nil {
				return err
			}
		}
		if member.SpendLimit > 0 {
			var used int64
			if err := tx.Model(&OrganizationCharge{}).Scopes(OrgScope(orgID)).Where("user_id = ? AND period_start = ? AND status IN ?", userID, period, []string{"reserved", "settled"}).Select("COALESCE(SUM(quota), 0)").Scan(&used).Error; err != nil {
				return err
			}
			if used > member.SpendLimit || delta > member.SpendLimit-used {
				return ErrMemberSpendLimit
			}
		}
		if receipt.Id != 0 {
			if receipt.SubscriptionId > 0 {
				result := tx.Model(&UserSubscription{}).Scopes(OrgScope(orgID)).Where("id = ? AND last_reset_time = ? AND (amount_total = 0 OR amount_used <= amount_total - ?)", receipt.SubscriptionId, receipt.SubscriptionPeriod, delta).Update("amount_used", gorm.Expr("amount_used + ?", delta))
				if result.Error != nil {
					return result.Error
				}
				if result.RowsAffected != 1 {
					return ErrOrganizationQuota
				}
			} else {
				if org.Quota < delta {
					return ErrOrganizationQuota
				}
				if err := tx.Model(&org).Update("quota", gorm.Expr("quota - ?", delta)).Error; err != nil {
					return err
				}
			}
			receipt.Quota = amount
			if err := tx.Model(&receipt).Update("quota", amount).Error; err != nil {
				return err
			}
			return syncPersonalOrganizationWalletTx(tx, orgID)
		}
		receipt = OrganizationCharge{RequestId: requestID, OrgId: orgID, UserId: userID, TokenId: tokenID, TokenQuotaManaged: manageToken, PeriodStart: period, Quota: amount, Status: "reserved"}
		preference := "subscription_first"
		if org.Kind == OrganizationPersonal && org.PersonalUserId != nil {
			var owner User
			if err := tx.Select("setting").Where("id = ?", *org.PersonalUserId).First(&owner).Error; err != nil {
				return err
			}
			preference = common.NormalizeBillingPreference(owner.GetSetting().BillingPreference)
		}
		allowWallet := preference != "subscription_only"
		walletPreferred := preference == "wallet_only" || preference == "wallet_first" && org.Quota >= amount
		for _, sub := range subs {
			if walletPreferred {
				break
			}
			if !sub.AllowWalletOverflow {
				allowWallet = false
			}
			if sub.AmountTotal > 0 && sub.AmountTotal-sub.AmountUsed < amount {
				continue
			}
			if sub.AmountUsed > int64(common.MaxWalletQuota)-amount {
				return ErrOrganizationQuota
			}
			if err := tx.Model(&sub).Update("amount_used", gorm.Expr("amount_used + ?", amount)).Error; err != nil {
				return err
			}
			receipt.SubscriptionId = sub.Id
			receipt.SubscriptionPeriod = sub.LastResetTime
			break
		}
		if receipt.SubscriptionId == 0 {
			if !allowWallet || org.Quota < amount {
				return ErrOrganizationQuota
			}
			if err := tx.Model(&org).Update("quota", gorm.Expr("quota - ?", amount)).Error; err != nil {
				return err
			}
		}
		if err := tx.Create(&receipt).Error; err != nil {
			return err
		}
		return syncPersonalOrganizationWalletTx(tx, orgID)
	})
	return &receipt, err
}

// FinalizeOrganizationCharge settles or refunds a durable reservation exactly
// once, including across retries, workers, and process restarts.
func FinalizeOrganizationCharge(orgID int, requestID string, actual int64, refund bool) error {
	if actual < 0 || actual > int64(common.MaxQuota) {
		return ErrOrganizationInput
	}
	if refund {
		actual = 0
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		return finalizeOrganizationChargeTx(tx, orgID, requestID, actual, refund, false)
	})
}

func finalizeOrganizationChargeTx(tx *gorm.DB, orgID int, requestID string, actual int64, refund, adjustment bool) error {
	var org Organization
	if err := lockForUpdate(tx).Where("id = ?", orgID).First(&org).Error; err != nil {
		return err
	}
	var receipt OrganizationCharge
	if err := tx.Scopes(OrgScope(orgID)).Where("request_id = ?", requestID).First(&receipt).Error; err != nil {
		return err
	}
	if receipt.Status != "reserved" && !(adjustment && receipt.Status == "settled") {
		if receipt.Status == "refunded" && refund || receipt.Status == "settled" && !refund && receipt.Quota == actual {
			return nil
		}
		return errors.New("request has already been finalized")
	}
	delta := actual - receipt.Quota
	if receipt.TokenQuotaManaged && receipt.TokenId > 0 {
		if err := adjustOrganizationTokenQuotaTx(tx, orgID, receipt.UserId, receipt.TokenId, delta, false); err != nil {
			return err
		}
	}
	if receipt.SubscriptionId > 0 {
		var sub UserSubscription
		if err := lockForUpdate(tx).Scopes(OrgScope(orgID)).Where("id = ?", receipt.SubscriptionId).First(&sub).Error; err != nil {
			return err
		}
		// The reservation belongs to its original reset period. Never credit
		// an expired allowance into the new period, or charge that period twice.
		periodDelta := delta
		if sub.LastResetTime != receipt.SubscriptionPeriod {
			periodDelta = 0
		}
		result := tx.Model(&UserSubscription{}).Scopes(OrgScope(orgID)).Where("id = ? AND amount_used >= ? AND amount_used <= ?", receipt.SubscriptionId, -periodDelta, int64(common.MaxWalletQuota)-periodDelta).Update("amount_used", gorm.Expr("amount_used + ?", periodDelta))
		if result.Error != nil {
			return result.Error
		}
		if periodDelta != 0 && result.RowsAffected != 1 {
			return ErrOrganizationQuota
		}
	} else {
		if delta > 0 && org.Quota < -int64(common.MaxWalletQuota)+delta || delta < 0 && org.Quota > int64(common.MaxWalletQuota)+delta {
			return ErrOrganizationQuota
		}
		if err := tx.Model(&org).Update("quota", gorm.Expr("quota - ?", delta)).Error; err != nil {
			return err
		}
	}
	usageDelta := actual
	if receipt.Status == "settled" {
		usageDelta -= receipt.Quota
	}
	if org.UsedQuota > int64(math.MaxInt64)-max(usageDelta, 0) || org.UsedQuota < -usageDelta {
		return errors.New("organization usage counter exceeds supported range")
	}
	if err := tx.Model(&org).Update("used_quota", gorm.Expr("used_quota + ?", usageDelta)).Error; err != nil {
		return err
	}

	status := "settled"
	if refund {
		status = "refunded"
	}
	if err := tx.Model(&receipt).Updates(map[string]interface{}{"quota": actual, "status": status}).Error; err != nil {
		return err
	}
	if !refund && usageDelta > 0 {
		if err := queueOrganizationBudgetNotificationsTx(tx, &org); err != nil {
			return err
		}
	}

	return syncPersonalOrganizationWalletTx(tx, orgID)
}

// adjustOrganizationTokenQuotaTx serializes a Key's hard limit with its wallet.
// Finalization may exceed an estimate, but never wraps either quota counter.
func adjustOrganizationTokenQuotaTx(tx *gorm.DB, orgID, userID, tokenID int, delta int64, reserve bool) error {
	var token Token
	query := lockForUpdate(tx).Scopes(OrgScope(orgID)).Where("id = ? AND user_id = ?", tokenID, userID)
	if !reserve {
		query = query.Unscoped()
	}
	if err := query.First(&token).Error; err != nil {
		if !reserve && errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return ErrOrganizationAccess
	}
	if reserve {
		if token.Status != common.TokenStatusEnabled || token.ExpiredTime != -1 && token.ExpiredTime < common.GetTimestamp() {
			return ErrOrganizationAccess
		}
		if !token.UnlimitedQuota && (token.RemainQuota <= 0 || int64(token.RemainQuota) < delta) {
			return ErrOrganizationTokenQuota
		}
	}
	remain, used := int64(token.RemainQuota), int64(token.UsedQuota)
	if delta > 0 && (remain < -int64(common.MaxWalletQuota)+delta || used > int64(common.MaxWalletQuota)-delta) || delta < 0 && (remain > int64(common.MaxWalletQuota)+delta || used < -delta) {
		return ErrOrganizationTokenQuota
	}
	return tx.Unscoped().Model(&token).Updates(map[string]interface{}{"remain_quota": gorm.Expr("remain_quota - ?", delta), "used_quota": gorm.Expr("used_quota + ?", delta), "accessed_time": common.GetTimestamp()}).Error
}
