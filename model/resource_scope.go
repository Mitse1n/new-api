package model

import "gorm.io/gorm"

// ResourceScope selects personal resources or resources in one organization.
// AllMembers widens organization reads only; personal reads always require UserID.
type ResourceScope struct {
	OrgID      int
	UserID     int
	AllMembers bool
}

func (scope ResourceScope) Apply(db *gorm.DB) *gorm.DB {
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
