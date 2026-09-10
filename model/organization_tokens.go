package model

import (
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// DisableOrganizationMemberTokensTx revokes all keys of a member before a
// governance transaction commits. Re-enabling membership never revives keys.
func DisableOrganizationMemberTokensTx(tx *gorm.DB, orgID, userID int) error {
	var tokens []Token
	if err := tx.Scopes(OrgScope(orgID)).Where("user_id = ? AND status <> ?", userID, common.TokenStatusDisabled).Select("id", "key").Find(&tokens).Error; err != nil {
		return err
	}
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
