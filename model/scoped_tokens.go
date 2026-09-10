package model

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func InsertScopedToken(token *Token) error {
	if token == nil || token.UserId <= 0 || token.OrgId < 0 {
		return ErrOrganizationInput
	}
	if token.OrgId == 0 {
		token.OrgStatus, token.OrgGroup, token.OrgSettings = 0, "", ""
		return token.Insert()
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var org Organization
		if err := lockForUpdate(tx).Where("id = ? AND status = ?", token.OrgId, OrganizationActive).First(&org).Error; err != nil {
			return ErrOrganizationAccess
		}
		var member OrganizationMember
		if err := tx.Scopes(OrgScope(org.Id)).Where("user_id = ? AND status = ?", token.UserId, OrganizationActive).First(&member).Error; err != nil {
			return ErrOrganizationAccess
		}
		token.OrgStatus, token.OrgGroup, token.OrgSettings = org.Status, org.Group, org.Settings
		if err := tx.Create(token).Error; err != nil {
			return err
		}
		return tx.Create(&OrganizationAudit{OrgId: org.Id, ActorId: token.UserId, Action: "token.create", ObjectId: fmt.Sprint(token.Id), Result: "success"}).Error
	})
}

// TokenScope always binds a key to its creator, regardless of role.
type TokenScope struct {
	OrgID  int
	UserID int
}

func (scope TokenScope) Apply(db *gorm.DB) *gorm.DB {
	return (ResourceScope{OrgID: scope.OrgID, UserID: scope.UserID}).Apply(db)
}

func GetScopedToken(scope TokenScope, id int) (*Token, error) {
	var token Token
	err := DB.Scopes(scope.Apply).Where("id = ?", id).First(&token).Error
	return &token, err
}

func ListScopedTokens(scope TokenScope, keyword, token string, offset, limit int) ([]*Token, int64, error) {
	query := DB.Model(&Token{}).Scopes(scope.Apply)
	if keyword != "" {
		pattern, err := sanitizeLikePattern(keyword)
		if err != nil {
			return nil, 0, err
		}
		query = query.Where("name LIKE ? ESCAPE '!'", pattern)
	}
	if token != "" {
		pattern, err := sanitizeLikePattern(strings.TrimPrefix(token, "sk-"))
		if err != nil {
			return nil, 0, err
		}
		query = query.Where("? LIKE ? ESCAPE '!'", clause.Column{Name: "key"}, pattern)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	tokens := make([]*Token, 0)
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	err := query.Order("id desc").Offset(offset).Limit(limit).Find(&tokens).Error
	return tokens, total, err
}

func DeleteScopedTokens(scope TokenScope, ids []int) (int64, error) {
	if len(ids) == 0 || len(ids) > 100 {
		return 0, ErrOrganizationInput
	}
	var count int64
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := scope.AuthorizeWrite(tx); err != nil {
			return err
		}
		var tokens []Token
		if err := tx.Scopes(scope.Apply).Where("id IN ?", ids).Find(&tokens).Error; err != nil {
			return err
		}
		unique := make(map[int]bool, len(ids))
		for _, id := range ids {
			unique[id] = true
		}
		if len(tokens) != len(unique) {
			return ErrOrganizationAccess
		}
		for _, token := range tokens {
			if err := invalidateTokenCacheForMutation(token.Key); err != nil {
				return err
			}
		}
		result := tx.Scopes(scope.Apply).Where("id IN ?", ids).Delete(&Token{})
		count = result.RowsAffected
		if result.Error != nil {
			return result.Error
		}
		if scope.OrgID == 0 {
			return nil
		}
		return tx.Create(&OrganizationAudit{OrgId: scope.OrgID, ActorId: scope.UserID, Action: "token.delete", ObjectId: fmt.Sprint(ids), Result: "success"}).Error
	})
	return count, err
}

func (scope TokenScope) AuthorizeWrite(tx *gorm.DB) error {
	if scope.UserID <= 0 || scope.OrgID < 0 {
		return ErrOrganizationAccess
	}
	if scope.OrgID == 0 {
		var user User
		return lockForUpdate(tx).Select("id").Where("id = ? AND status = ?", scope.UserID, common.UserStatusEnabled).First(&user).Error
	}
	var org Organization
	if err := lockForUpdate(tx).Where("id = ? AND status = ?", scope.OrgID, OrganizationActive).First(&org).Error; err != nil {
		return ErrOrganizationAccess
	}
	var member OrganizationMember
	if err := tx.Scopes(OrgScope(org.Id)).Where("user_id = ? AND status = ?", scope.UserID, OrganizationActive).First(&member).Error; err != nil {
		return ErrOrganizationAccess
	}
	return nil
}

func UpdateScopedToken(scope TokenScope, token *Token, statusOnly bool) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := scope.AuthorizeWrite(tx); err != nil {
			return err
		}
		var current Token
		if err := tx.Scopes(scope.Apply).Where("id = ?", token.Id).First(&current).Error; err != nil {
			return ErrOrganizationAccess
		}
		if err := invalidateTokenCacheForMutation(current.Key); err != nil {
			return err
		}
		fields := []string{"name", "status", "expired_time", "remain_quota", "unlimited_quota", "model_limits_enabled", "model_limits", "allow_ips", "group", "cross_group_retry", "auto_groups"}
		if statusOnly {
			fields = []string{"status"}
		}
		if err := tx.Model(&current).Select(fields).Updates(token).Error; err != nil {
			return err
		}
		if scope.OrgID == 0 {
			return nil
		}
		return tx.Create(&OrganizationAudit{OrgId: scope.OrgID, ActorId: scope.UserID, Action: "token.update", ObjectId: fmt.Sprint(token.Id), Result: "success"}).Error
	})
}
