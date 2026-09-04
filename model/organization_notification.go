package model

import (
	"crypto/sha256"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// OrganizationNotification is a durable delivery queue. A recipient is queued
// once per budget period, independently of request retries and replica count.
type OrganizationNotification struct {
	Id            int    `json:"id"`
	OrgId         int    `json:"org_id" gorm:"index:idx_org_notification,priority:1;not null"`
	EventKey      string `json:"-" gorm:"type:varchar(64);uniqueIndex;not null"`
	Channel       string `json:"channel" gorm:"type:varchar(16)"`
	Destination   string `json:"-" gorm:"type:text"`
	Content       string `json:"-" gorm:"type:text"`
	Status        string `json:"status" gorm:"type:varchar(16);index:idx_notification_delivery,priority:1"`
	Attempts      int    `json:"attempts"`
	NextAttemptAt int64  `json:"next_attempt_at" gorm:"index:idx_notification_delivery,priority:2"`
	CreatedAt     int64  `json:"created_at" gorm:"autoCreateTime"`
}

func queueOrganizationBudgetNotificationsTx(tx *gorm.DB, org *Organization) error {
	settings, err := org.EffectiveSettings()
	if err != nil {
		return err
	}
	if settings.BudgetLimit <= 0 {
		return nil
	}
	var used int64
	if err := tx.Model(&OrganizationCharge{}).Scopes(OrgScope(org.Id)).Where("period_start = ? AND status = ?", org.BudgetPeriodStart, "settled").Select("COALESCE(SUM(quota), 0)").Scan(&used).Error; err != nil {
		return err
	}
	// Division avoids overflowing the wallet-sized budget when multiplying by 100.
	threshold := settings.BudgetLimit/100*int64(settings.AlertPercent) + (settings.BudgetLimit%100*int64(settings.AlertPercent)+99)/100
	if used < threshold {
		return nil
	}
	var emails []string
	if err := tx.Model(&OrganizationMember{}).Joins("JOIN users ON users.id = organization_members.user_id").Where("organization_members.org_id = ? AND organization_members.status = ? AND organization_members.role IN ? AND users.status = ?", org.Id, OrganizationActive, []string{OrgRoleOwner, OrgRoleAdmin}, common.UserStatusEnabled).Pluck("users.email", &emails).Error; err != nil {
		return err
	}
	destinations := map[string]string{}
	for _, email := range append(emails, settings.AlertEmail) {
		if email != "" {
			destinations["email:"+email] = email
		}
	}
	if settings.Webhook != "" {
		destinations["webhook:"+settings.Webhook] = settings.Webhook
	}
	content := fmt.Sprintf("%s: organization budget usage has reached %d%%. / 组织预算使用已达到 %d%%。", org.Name, settings.AlertPercent, settings.AlertPercent)
	for key, destination := range destinations {
		channel := "email"
		if key == "webhook:"+settings.Webhook {
			channel = "webhook"
		}
		hash := sha256.Sum256([]byte(fmt.Sprintf("%d:%d:%s", org.Id, org.BudgetPeriodStart, key)))
		notification := OrganizationNotification{OrgId: org.Id, EventKey: fmt.Sprintf("%x", hash), Channel: channel, Destination: destination, Content: content, Status: "pending", NextAttemptAt: common.GetTimestamp()}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&notification).Error; err != nil {
			return err
		}
	}
	return nil
}
