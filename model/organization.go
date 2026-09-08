package model

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	OrganizationPersonal = "personal"
	OrganizationTeam     = "team"
	OrganizationActive   = 1
	OrganizationDisabled = 2
	OrganizationDeleting = 3
	OrgRoleOwner         = "owner"
	OrgRoleAdmin         = "admin"
	OrgRoleMember        = "member"
)

var (
	ErrOrganizationAccess   = errors.New("organization access unavailable")
	ErrOrganizationSlug     = errors.New("organization slug already exists")
	ErrOrganizationInput    = errors.New("invalid organization details")
	ErrOrganizationOwner    = errors.New("organization ownership operation is not allowed")
	ErrOrganizationSeats    = errors.New("organization member limit reached")
	organizationSlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
)

// Organization is the tenant and billing owner. PersonalUserId is nullable so
// the database enforces one personal organization per user on every dialect.
type Organization struct {
	Id                int            `json:"id"`
	Name              string         `json:"name" gorm:"type:varchar(64);not null"`
	Slug              string         `json:"slug" gorm:"type:varchar(64);uniqueIndex;not null"`
	OwnerId           int            `json:"owner_id" gorm:"index"`
	PersonalUserId    *int           `json:"-" gorm:"uniqueIndex"`
	Kind              string         `json:"kind" gorm:"type:varchar(16);not null"`
	Status            int            `json:"status" gorm:"not null"`
	Group             string         `json:"group" gorm:"type:varchar(64);not null"`
	Quota             int64          `json:"quota" gorm:"type:bigint;not null"`
	UsedQuota         int64          `json:"used_quota" gorm:"type:bigint;not null"`
	Settings          string         `json:"settings" gorm:"type:text"`
	Version           int64          `json:"version" gorm:"type:bigint;not null"`
	BudgetPeriodStart int64          `json:"budget_period_start" gorm:"type:bigint"`
	BudgetPeriodEnd   int64          `json:"budget_period_end" gorm:"type:bigint"`
	CreatedAt         int64          `json:"created_at" gorm:"autoCreateTime"`
	DeletedAt         gorm.DeletedAt `json:"-" gorm:"index"`
}

type OrganizationMember struct {
	Id         int    `json:"id"`
	OrgId      int    `json:"org_id" gorm:"uniqueIndex:idx_org_member,priority:1;not null"`
	UserId     int    `json:"user_id" gorm:"uniqueIndex:idx_org_member,priority:2;index;not null"`
	Role       string `json:"role" gorm:"type:varchar(16);not null"`
	SpendLimit int64  `json:"spend_limit" gorm:"type:bigint;not null"`
	Status     int    `json:"status" gorm:"not null"`
	CreatedAt  int64  `json:"created_at" gorm:"autoCreateTime"`
}

type OrganizationInvite struct {
	Id         int    `json:"id"`
	OrgId      int    `json:"org_id" gorm:"index:idx_org_invite;not null"`
	Username   string `json:"username" gorm:"type:varchar(64)"`
	InviteeId  int    `json:"invitee_id" gorm:"index:idx_org_invite_recipient,priority:1"`
	Role       string `json:"role" gorm:"type:varchar(16);not null"`
	Status     string `json:"status" gorm:"type:varchar(16);not null;index:idx_org_invite_recipient,priority:2"`
	InviterId  int    `json:"inviter_id"`
	AcceptedBy int    `json:"accepted_by"`
	ExpiresAt  int64  `json:"expires_at" gorm:"type:bigint;index:idx_org_invite_recipient,priority:3"`
	CreatedAt  int64  `json:"created_at" gorm:"autoCreateTime"`
}

// Transfers require the target's explicit acceptance; merely sending an
// invitation never changes either member's role.
type OrganizationTransfer struct {
	Id        int   `json:"id"`
	OrgId     int   `json:"org_id" gorm:"uniqueIndex;not null"`
	OwnerId   int   `json:"owner_id"`
	TargetId  int   `json:"target_id"`
	ExpiresAt int64 `json:"expires_at" gorm:"type:bigint"`
}

type OrganizationAudit struct {
	Reason    string `json:"reason,omitempty" gorm:"type:text"`
	Id        int    `json:"id"`
	OrgId     int    `json:"org_id" gorm:"index:idx_org_audit,priority:1;not null"`
	ActorId   int    `json:"actor_id"`
	Action    string `json:"action" gorm:"type:varchar(64)"`
	ObjectId  string `json:"object_id" gorm:"type:varchar(128)"`
	Result    string `json:"result" gorm:"type:varchar(32)"`
	CreatedAt int64  `json:"created_at" gorm:"autoCreateTime;index:idx_org_audit,priority:2"`
}

// OrgScope fails closed for missing context. Platform queries must use a
// separate, explicitly named entry point rather than treating zero as global.
func OrgScope(orgID int) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if orgID <= 0 {
			return db.Where("1 = 0")
		}
		return db.Where("org_id = ?", orgID)
	}
}

func EnsurePersonalOrganization(tx *gorm.DB, user *User) (*Organization, error) {
	if user == nil || user.Id <= 0 {
		return nil, ErrOrganizationInput
	}
	var org Organization
	err := tx.Unscoped().Where("personal_user_id = ?", user.Id).First(&org).Error
	if err == nil {
		user.PersonalOrgId = org.Id
		return &org, tx.Model(&User{}).Where("id = ?", user.Id).Update("personal_org_id", org.Id).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	name := user.DisplayName
	if name == "" {
		name = user.Username
	}
	org = Organization{Name: name, Slug: fmt.Sprintf("personal-%d", user.Id), OwnerId: user.Id,
		PersonalUserId: &user.Id, Kind: OrganizationPersonal, Status: OrganizationActive,
		Group: user.Group, Quota: int64(user.Quota), UsedQuota: int64(user.UsedQuota), Version: 1}
	if org.Group == "" {
		org.Group = "default"
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&org).Error; err != nil {
		return nil, err
	}
	if err := tx.Unscoped().Where("personal_user_id = ?", user.Id).First(&org).Error; err != nil {
		return nil, err
	}
	member := OrganizationMember{OrgId: org.Id, UserId: user.Id, Role: OrgRoleOwner, Status: OrganizationActive}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&member).Error; err != nil {
		return nil, err
	}
	user.PersonalOrgId = org.Id
	return &org, tx.Model(&User{}).Where("id = ?", user.Id).Update("personal_org_id", org.Id).Error
}

func GetPersonalOrganization(userID int) (*Organization, error) {
	var org Organization
	err := DB.Where("personal_user_id = ?", userID).First(&org).Error
	return &org, err
}

func CreateUserWithPersonalOrganization(user *User) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(user).Error; err != nil {
			return err
		}
		_, err := EnsurePersonalOrganization(tx, user)
		return err
	})
}

func GetOrganizationMembership(orgID, userID int) (*Organization, *OrganizationMember, error) {
	if orgID <= 0 || userID <= 0 {
		return nil, nil, ErrOrganizationAccess
	}
	var member OrganizationMember
	if err := DB.Scopes(OrgScope(orgID)).Where("user_id = ? AND status = ?", userID, OrganizationActive).First(&member).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrOrganizationAccess
		}
		return nil, nil, err
	}
	var org Organization
	if err := DB.Where("id = ? AND status = ?", orgID, OrganizationActive).First(&org).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrOrganizationAccess
		}
		return nil, nil, err
	}
	return &org, &member, nil
}

func CreateTeamOrganization(userID int, name, slug string) (*Organization, error) {
	name, slug = strings.TrimSpace(name), strings.ToLower(strings.TrimSpace(slug))
	if userID <= 0 || name == "" || utf8.RuneCountInString(name) > 64 || len(slug) > 64 ||
		!organizationSlugPattern.MatchString(slug) || strings.HasPrefix(slug, "personal-") {
		return nil, ErrOrganizationInput
	}
	org := Organization{Name: name, Slug: slug, OwnerId: userID, Kind: OrganizationTeam,
		Status: OrganizationActive, Group: "default", Version: 1}
	err := DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := tx.Where("id = ? AND status = ?", userID, common.UserStatusEnabled).First(&user).Error; err != nil {
			return err
		}
		if err := tx.Create(&org).Error; err != nil {
			return err
		}
		if err := tx.Create(&OrganizationMember{OrgId: org.Id, UserId: userID, Role: OrgRoleOwner, Status: OrganizationActive}).Error; err != nil {
			return err
		}
		return tx.Create(&OrganizationAudit{OrgId: org.Id, ActorId: userID, Action: "organization.create", ObjectId: fmt.Sprint(org.Id), Result: "success"}).Error
	})
	if err != nil {
		var duplicate int64
		if lookupErr := DB.Unscoped().Model(&Organization{}).Where("slug = ?", slug).Count(&duplicate).Error; lookupErr == nil && duplicate > 0 {
			return nil, ErrOrganizationSlug
		}
	}
	return &org, err
}

// RecordOrganizationRequestFailure records only a validated organization context and
// the route template; request bodies and invitation/key secrets are excluded.
func RecordOrganizationRequestFailure(orgID, actorID, status int, route string) {
	if orgID <= 0 || actorID <= 0 {
		return
	}
	if len(route) > 128 {
		route = route[:128]
	}
	result := "failed"
	if status == 403 {
		result = "denied"
	}
	err := DB.Create(&OrganizationAudit{OrgId: orgID, ActorId: actorID, Action: "request.failed", ObjectId: route, Result: result, Reason: fmt.Sprint(status)}).Error
	if err != nil {
		common.SysError("organization failure audit: " + err.Error())
	}
}
