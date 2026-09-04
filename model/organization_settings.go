package model

import (
	"fmt"
	"net/mail"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// OrganizationSettings deliberately excludes pricing, routing groups and credentials.
// AllowedModels is intersected with the token and platform grant at request time.
type OrganizationSettings struct {
	Logo              string   `json:"logo"`
	AlertEmail        string   `json:"alert_email"`
	Webhook           string   `json:"webhook"`
	DefaultSpendLimit int64    `json:"default_spend_limit"`
	AllowedModels     []string `json:"allowed_models"`
	AlertPercent      int      `json:"alert_percent"`
	BudgetLimit       int64    `json:"budget_limit"`
}

func (org *Organization) EffectiveSettings() (OrganizationSettings, error) {
	settings := OrganizationSettings{AlertPercent: 80, AllowedModels: []string{}}
	if org.Settings != "" {
		if err := common.UnmarshalJsonStr(org.Settings, &settings); err != nil {
			return settings, err
		}
	}
	return settings, nil
}

func UpdateOrganizationSettings(orgID, actorID int, name string, settings OrganizationSettings) error {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 64 || settings.DefaultSpendLimit < 0 || settings.DefaultSpendLimit > int64(common.MaxWalletQuota) || settings.BudgetLimit < 0 || settings.BudgetLimit > int64(common.MaxWalletQuota) || settings.AlertPercent < 1 || settings.AlertPercent > 100 || len(settings.AllowedModels) > 1000 {
		return ErrOrganizationInput
	}
	for _, raw := range []string{settings.Logo, settings.Webhook} {
		if raw == "" {
			continue
		}
		uri, err := url.Parse(raw)
		if err != nil || uri.Scheme != "https" || uri.Hostname() == "" || uri.User != nil || len(raw) > 2048 {
			return ErrOrganizationInput
		}
	}
	if settings.AlertEmail != "" {
		address, err := mail.ParseAddress(settings.AlertEmail)
		if err != nil || address.Address != settings.AlertEmail || len(settings.AlertEmail) > 254 {
			return ErrOrganizationInput
		}
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		org, err := lockOrganizationManager(tx, orgID, actorID, false)
		if err != nil {
			return err
		}
		permitted := map[string]bool{}
		for _, name := range GetGroupEnabledModels(org.Group) {
			permitted[name] = true
		}
		for _, name := range settings.AllowedModels {
			if !permitted[name] {
				return ErrOrganizationInput
			}
		}
		data, err := common.Marshal(settings)
		if err != nil {
			return err
		}
		org.Name, org.Settings = name, string(data)
		org.Version++
		if err := tx.Model(org).Updates(map[string]interface{}{"name": name, "settings": org.Settings, "version": org.Version}).Error; err != nil {
			return err
		}
		if err := RefreshOrganizationTokensTx(tx, org); err != nil {
			return err
		}
		return tx.Create(&OrganizationAudit{OrgId: orgID, ActorId: actorID, Action: "settings.update", ObjectId: fmt.Sprint(orgID), Result: "success"}).Error
	})
}

func SetOrganizationMemberBudget(orgID, actorID, userID int, limit int64) error {
	if limit < 0 || limit > int64(common.MaxWalletQuota) {
		return ErrOrganizationInput
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if _, err := lockOrganizationManager(tx, orgID, actorID, false); err != nil {
			return err
		}
		var member OrganizationMember
		if err := tx.Scopes(OrgScope(orgID)).Where("user_id = ?", userID).First(&member).Error; err != nil {
			return ErrOrganizationAccess
		}
		if err := tx.Model(&member).Update("spend_limit", limit).Error; err != nil {
			return err
		}
		return tx.Create(&OrganizationAudit{OrgId: orgID, ActorId: actorID, Action: "member.budget", ObjectId: fmt.Sprint(userID), Result: "success"}).Error
	})
}
