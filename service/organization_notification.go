package service

import (
	"fmt"
	"html"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"gorm.io/gorm"
)

func DeliverOrganizationNotifications() error {
	var pending []model.OrganizationNotification
	now := common.GetTimestamp()
	if err := model.DB.Where("status = ? AND next_attempt_at <= ?", "pending", now).Order("id").Limit(100).Find(&pending).Error; err != nil {
		return err
	}
	for _, item := range pending {
		// A conditional lease permits safe retries after a worker crashes. Network
		// delivery is at-least-once; the durable event id identifies duplicate webhooks.
		claimed := model.DB.Model(&model.OrganizationNotification{}).Where("id = ? AND status = ? AND next_attempt_at = ?", item.Id, "pending", item.NextAttemptAt).Updates(map[string]interface{}{"next_attempt_at": now + 300, "attempts": gorm.Expr("attempts + 1")})
		if claimed.Error != nil {
			return claimed.Error
		}
		if claimed.RowsAffected != 1 {
			continue
		}
		var org model.Organization
		err := model.DB.Where("id = ?", item.OrgId).First(&org).Error
		if err == nil {
			if item.Channel == "email" {
				err = common.SendEmail("New API — Organization budget alert / 组织预算告警", item.Destination, html.EscapeString(item.Content))
			} else {
				// Validate even when the global worker transport is enabled.
				err = ValidateSSRFProtectedFetchURL(item.Destination)
				if err == nil {
					err = SendWebhookNotify(item.Destination, "", dto.NewNotify("organization.budget", "Organization budget alert", item.Content+fmt.Sprintf(" [org-budget-%d]", item.Id), nil))
				}
			}
		}
		status := "sent"
		if err != nil {
			status = "pending"
			if item.Attempts >= 7 {
				status = "failed"
			}
			common.SysError(fmt.Sprintf("organization notification %d delivery failed: %v", item.Id, err))
		}
		if err := model.DB.Model(&model.OrganizationNotification{}).Where("id = ?", item.Id).Updates(map[string]interface{}{"status": status, "next_attempt_at": time.Now().Add(time.Minute * time.Duration(min(item.Attempts+1, 5))).Unix()}).Error; err != nil {
			return err
		}
	}
	return nil
}
