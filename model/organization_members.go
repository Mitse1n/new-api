package model

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

var ErrOrganizationInvite = errors.New("invitation unavailable or identity does not match")
var ErrOrganizationMemberExists = errors.New("user is already an active organization member")
var ErrOrganizationInvitePending = errors.New("an active invitation already exists")

type OrganizationMembership struct {
	Organization
	Role       string `json:"role"`
	SpendLimit int64  `json:"spend_limit"`
}

func ListUserOrganizations(userID int) ([]OrganizationMembership, error) {
	orgs := make([]OrganizationMembership, 0)
	err := DB.Model(&Organization{}).Select("organizations.*, organization_members.role, organization_members.spend_limit").
		Joins("JOIN organization_members ON organization_members.org_id = organizations.id").
		Where("organization_members.user_id = ? AND organization_members.status = ? AND (organizations.status = ? OR (organizations.status = ? AND organizations.owner_id = ?))", userID, OrganizationActive, OrganizationActive, OrganizationDisabled, userID).
		Order("organizations.id").Scan(&orgs).Error
	for i := range orgs {
		// The switcher never needs private notification destinations or team totals.
		orgs[i].Settings = ""
		if orgs[i].Role == OrgRoleMember {
			orgs[i].UsedQuota = 0
			orgs[i].Quota = 0
		}
	}
	return orgs, err
}

// lockOrganizationManager serializes governance with checkout and member
// acceptance. Authority is rechecked inside the same transaction as the write.
func lockOrganizationManager(tx *gorm.DB, orgID, actorID int, ownerOnly bool) (*Organization, error) {
	var org Organization
	if err := lockForUpdate(tx).Where("id = ? AND status = ?", orgID, OrganizationActive).First(&org).Error; err != nil {
		return nil, ErrOrganizationAccess
	}
	var member OrganizationMember
	if err := tx.Scopes(OrgScope(orgID)).Where("user_id = ? AND status = ?", actorID, OrganizationActive).First(&member).Error; err != nil {
		return nil, ErrOrganizationAccess
	}
	if member.Role != OrgRoleOwner && (ownerOnly || member.Role != OrgRoleAdmin) {
		return nil, ErrOrganizationAccess
	}
	return &org, nil
}

func organizationSeatLimit(tx *gorm.DB, orgID int) (int, error) {
	var subs []UserSubscription
	err := tx.Scopes(OrgScope(orgID)).Where("status = ? AND end_time > ?", "active", common.GetTimestamp()).Find(&subs).Error
	if err != nil {
		return 0, err
	}
	max := 0
	for _, sub := range subs {
		plan, err := GetOrganizationSubscriptionPlan(tx, &sub)
		if err != nil {
			return 0, err
		}
		if plan.MaxMembers == 0 {
			return 0, nil
		}
		if plan.MaxMembers > max {
			max = plan.MaxMembers
		}
	}
	return max, nil
}

func CreateOrganizationInvite(orgID, actorID int, email, role string) (*OrganizationInvite, string, error) {
	email = NormalizeEmail(email)
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || len(email) > 254 || (role != OrgRoleAdmin && role != OrgRoleMember) {
		return nil, "", ErrOrganizationInput
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, "", err
	}
	token := hex.EncodeToString(secret)
	hash := sha256.Sum256([]byte(token))
	invite := OrganizationInvite{OrgId: orgID, Email: email, Role: role, TokenHash: hex.EncodeToString(hash[:]), Status: "pending", InviterId: actorID, ExpiresAt: time.Now().Add(7 * 24 * time.Hour).Unix()}
	err = DB.Transaction(func(tx *gorm.DB) error {
		org, err := lockOrganizationManager(tx, orgID, actorID, false)
		if err != nil {
			return err
		}
		if org.Kind == OrganizationPersonal {
			return ErrOrganizationOwner
		}
		var count int64
		if err := tx.Model(&OrganizationMember{}).Joins("JOIN users ON users.id = organization_members.user_id").Where("organization_members.org_id = ? AND organization_members.status = ? AND LOWER(users.email) = ?", orgID, OrganizationActive, email).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrOrganizationMemberExists
		}
		if err := tx.Model(&OrganizationInvite{}).Scopes(OrgScope(orgID)).Where("email = ? AND status = ? AND expires_at > ?", email, "pending", common.GetTimestamp()).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrOrganizationInvitePending
		}
		max, err := organizationSeatLimit(tx, orgID)
		if err != nil {
			return err
		}
		if max > 0 {
			var pending int64
			if err := tx.Model(&OrganizationMember{}).Scopes(OrgScope(orgID)).Where("status = ?", OrganizationActive).Count(&count).Error; err != nil {
				return err
			}
			if err := tx.Model(&OrganizationInvite{}).Scopes(OrgScope(orgID)).Where("status = ? AND expires_at > ?", "pending", common.GetTimestamp()).Count(&pending).Error; err != nil {
				return err
			}
			if count+pending >= int64(max) {
				return ErrOrganizationSeats
			}
		}
		if err := tx.Create(&invite).Error; err != nil {
			return err
		}
		return tx.Create(&OrganizationAudit{OrgId: orgID, ActorId: actorID, Action: "member.invite", ObjectId: fmt.Sprint(invite.Id), Result: "success"}).Error
	})
	return &invite, token, err
}

func AcceptOrganizationInvite(userID int, token string) (int, error) {
	if len(token) != 64 {
		return 0, ErrOrganizationInvite
	}
	hash := sha256.Sum256([]byte(token))
	var acceptedOrgID int
	err := DB.Transaction(func(tx *gorm.DB) error {
		var invite OrganizationInvite
		if err := tx.Where("token_hash = ?", hex.EncodeToString(hash[:])).First(&invite).Error; err != nil {
			return ErrOrganizationInvite
		}
		var org Organization
		if err := lockForUpdate(tx).Where("id = ? AND status = ?", invite.OrgId, OrganizationActive).First(&org).Error; err != nil {
			return ErrOrganizationInvite
		}
		// Reload after acquiring the organization lock to serialize acceptance,
		// revocation and concurrent invitations.
		if err := tx.First(&invite, invite.Id).Error; err != nil {
			return err
		}
		if invite.Status == "accepted" && invite.AcceptedBy == userID {
			acceptedOrgID = invite.OrgId
			return nil
		}
		if invite.Status != "pending" || invite.ExpiresAt <= common.GetTimestamp() {
			return ErrOrganizationInvite
		}
		var user User
		if err := tx.Where("id = ? AND status = ?", userID, common.UserStatusEnabled).First(&user).Error; err != nil {
			return ErrOrganizationInvite
		}
		if NormalizeEmail(user.Email) != invite.Email {
			return ErrOrganizationInvite
		}
		var member OrganizationMember
		err := tx.Scopes(OrgScope(org.Id)).Where("user_id = ?", userID).First(&member).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if member.Id != 0 && member.Status == OrganizationActive {
			return ErrOrganizationInvite
		}
		max, err := organizationSeatLimit(tx, org.Id)
		if err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&OrganizationMember{}).Scopes(OrgScope(org.Id)).Where("status = ?", OrganizationActive).Count(&count).Error; err != nil {
			return err
		}
		if max > 0 && count >= int64(max) {
			return ErrOrganizationSeats
		}
		settings, err := org.EffectiveSettings()
		if err != nil {
			return err
		}
		if member.Id == 0 {
			member.SpendLimit = settings.DefaultSpendLimit
		}
		member.OrgId, member.UserId, member.Role, member.Status = org.Id, userID, invite.Role, OrganizationActive
		if err := tx.Save(&member).Error; err != nil {
			return err
		}
		if err := tx.Model(&invite).Updates(map[string]interface{}{"status": "accepted", "accepted_by": userID}).Error; err != nil {
			return err
		}
		acceptedOrgID = org.Id
		return tx.Create(&OrganizationAudit{OrgId: org.Id, ActorId: userID, Action: "member.accept", ObjectId: fmt.Sprint(member.Id), Result: "success"}).Error
	})
	return acceptedOrgID, err
}

func UpdateOrganizationMember(orgID, actorID, userID int, role string, status int, spendLimit int64) error {
	if (role != OrgRoleAdmin && role != OrgRoleMember) || (status != OrganizationActive && status != OrganizationDisabled && status != OrganizationDeleting) || spendLimit < 0 || spendLimit > int64(common.MaxWalletQuota) {
		return ErrOrganizationInput
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		org, err := lockOrganizationManager(tx, orgID, actorID, false)
		if err != nil {
			return err
		}
		if org.Kind == OrganizationPersonal {
			return ErrOrganizationOwner
		}
		var member OrganizationMember
		if err := tx.Scopes(OrgScope(orgID)).Where("user_id = ?", userID).First(&member).Error; err != nil {
			return ErrOrganizationAccess
		}
		if member.Role == OrgRoleOwner {
			return ErrOrganizationOwner
		}
		if status == OrganizationActive && member.Status != OrganizationActive {
			max, err := organizationSeatLimit(tx, orgID)
			if err != nil {
				return err
			}
			var count, pending int64
			if err := tx.Model(&OrganizationMember{}).Scopes(OrgScope(orgID)).Where("status = ?", OrganizationActive).Count(&count).Error; err != nil {
				return err
			}
			if err := tx.Model(&OrganizationInvite{}).Scopes(OrgScope(orgID)).Where("status = ? AND expires_at > ?", "pending", common.GetTimestamp()).Count(&pending).Error; err != nil {
				return err
			}
			if max > 0 && count+pending >= int64(max) {
				return ErrOrganizationSeats
			}
		}
		if err := tx.Model(&member).Updates(map[string]interface{}{"role": role, "status": status, "spend_limit": spendLimit}).Error; err != nil {
			return err
		}
		return tx.Create(&OrganizationAudit{OrgId: orgID, ActorId: actorID, Action: "member.update", ObjectId: fmt.Sprint(userID), Result: "success"}).Error
	})
}

func RevokeOrganizationInvite(orgID, actorID, inviteID int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if _, err := lockOrganizationManager(tx, orgID, actorID, false); err != nil {
			return err
		}
		result := tx.Model(&OrganizationInvite{}).Scopes(OrgScope(orgID)).Where("id = ? AND status = ?", inviteID, "pending").Update("status", "revoked")
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrOrganizationInvite
		}
		return tx.Create(&OrganizationAudit{OrgId: orgID, ActorId: actorID, Action: "invite.revoke", ObjectId: fmt.Sprint(inviteID), Result: "success"}).Error
	})
}

// Resending rotates the hash on the existing invitation, invalidating the old
// link while retaining a single pending seat and an auditable invitation id.
func ResendOrganizationInvite(orgID, actorID, inviteID int) (*OrganizationInvite, string, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, "", err
	}
	token := hex.EncodeToString(secret)
	hash := sha256.Sum256([]byte(token))
	var invite OrganizationInvite
	err := DB.Transaction(func(tx *gorm.DB) error {
		if _, err := lockOrganizationManager(tx, orgID, actorID, false); err != nil {
			return err
		}
		if err := tx.Scopes(OrgScope(orgID)).Where("id = ? AND status = ?", inviteID, "pending").First(&invite).Error; err != nil {
			return ErrOrganizationInvite
		}
		if invite.ExpiresAt <= common.GetTimestamp() {
			max, err := organizationSeatLimit(tx, orgID)
			if err != nil {
				return err
			}
			var members, pending int64
			if err := tx.Model(&OrganizationMember{}).Scopes(OrgScope(orgID)).Where("status = ?", OrganizationActive).Count(&members).Error; err != nil {
				return err
			}
			if err := tx.Model(&OrganizationInvite{}).Scopes(OrgScope(orgID)).Where("status = ? AND expires_at > ?", "pending", common.GetTimestamp()).Count(&pending).Error; err != nil {
				return err
			}
			if max > 0 && members+pending >= int64(max) {
				return ErrOrganizationSeats
			}
			if err := tx.Model(&OrganizationInvite{}).Scopes(OrgScope(orgID)).Where("email = ? AND id <> ? AND status = ? AND expires_at > ?", invite.Email, invite.Id, "pending", common.GetTimestamp()).Count(&pending).Error; err != nil {
				return err
			}
			if pending > 0 {
				return ErrOrganizationInvite
			}
		}
		invite.TokenHash, invite.ExpiresAt = hex.EncodeToString(hash[:]), time.Now().Add(7*24*time.Hour).Unix()
		if err := tx.Save(&invite).Error; err != nil {
			return err
		}
		return tx.Create(&OrganizationAudit{OrgId: orgID, ActorId: actorID, Action: "invite.resend", ObjectId: fmt.Sprint(invite.Id), Result: "success"}).Error
	})
	return &invite, token, err
}
