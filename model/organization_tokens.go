package model

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

func InsertOrganizationToken(token *Token) error {
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

// Organization state is projected into tokens so resolving an organization never
// adds a database lookup to relay authentication. Governance writes invalidate
// cached projections before committing, including on other Redis clients.
func RefreshOrganizationTokensTx(tx *gorm.DB, org *Organization) error {
	var tokens []Token
	if err := tx.Scopes(OrgScope(org.Id)).Select("id", "key").Find(&tokens).Error; err != nil {
		return err
	}
	for _, token := range tokens {
		if err := invalidateTokenCacheForMutation(token.Key); err != nil {
			return err
		}
	}
	return tx.Model(&Token{}).Scopes(OrgScope(org.Id)).Updates(map[string]interface{}{
		"org_status": org.Status, "org_group": org.Group, "org_settings": org.Settings,
	}).Error
}

type OrganizationResourceScope struct {
	OrgID      int
	UserID     int
	AllMembers bool
}

func (scope OrganizationResourceScope) Apply(db *gorm.DB) *gorm.DB {
	if scope.OrgID < 0 || scope.UserID <= 0 && (scope.OrgID == 0 || !scope.AllMembers) {
		return db.Where("1 = 0")
	}
	if scope.OrgID == 0 {
		return db.Where("(org_id IS NULL OR org_id = 0) AND user_id = ?", scope.UserID)
	}
	db = db.Scopes(OrgScope(scope.OrgID))
	if !scope.AllMembers {
		db = db.Where("user_id = ?", scope.UserID)
	}
	return db
}

// OrganizationTokenScope always binds a key to its creator, regardless of role.
type OrganizationTokenScope struct {
	OrgID  int
	UserID int
}

func (scope OrganizationTokenScope) Apply(db *gorm.DB) *gorm.DB {
	return (OrganizationResourceScope{OrgID: scope.OrgID, UserID: scope.UserID}).Apply(db)
}

func GetOrganizationToken(scope OrganizationTokenScope, id int) (*Token, error) {
	var token Token
	err := DB.Scopes(scope.Apply).Where("id = ?", id).First(&token).Error
	return &token, err
}

func ListOrganizationTokens(scope OrganizationTokenScope, keyword string, offset, limit int) ([]*Token, int64, error) {
	query := DB.Model(&Token{}).Scopes(scope.Apply)
	if keyword != "" {
		pattern, err := sanitizeLikePattern(keyword)
		if err != nil {
			return nil, 0, err
		}
		query = query.Where("name LIKE ? ESCAPE '!'", pattern)
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

func DeleteOrganizationTokens(scope OrganizationTokenScope, ids []int) (int64, error) {
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

func (scope OrganizationTokenScope) AuthorizeWrite(tx *gorm.DB) error {
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

func UpdateOrganizationToken(scope OrganizationTokenScope, token *Token, statusOnly bool) error {
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

// DisableOrganizationMemberTokensTx revokes all keys of a member before a
// governance transaction commits. Re-enabling membership never revives keys.
func DisableOrganizationMemberTokensTx(tx *gorm.DB, orgID, userID int) error {
	var tokens []Token
	if err := tx.Scopes(OrgScope(orgID)).Where("user_id = ? AND status <> ?", userID, common.TokenStatusDisabled).Select("id", "key").Find(&tokens).Error; err != nil {
		return err
	}
	return disableOrganizationTokensTx(tx, tokens)
}

func disableOrganizationTokensTx(tx *gorm.DB, tokens []Token) error {
	if len(tokens) == 0 {
		return nil
	}
	ids := make([]int, 0, len(tokens))
	for _, token := range tokens {
		if err := invalidateTokenCacheForMutation(token.Key); err != nil {
			return err
		}
		ids = append(ids, token.Id)
	}
	return tx.Model(&Token{}).Where("id IN ?", ids).Update("status", common.TokenStatusDisabled).Error
}
