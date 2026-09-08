package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccountTokenLifecycleDoesNotRequireOrganization(t *testing.T) {
	db := organizationTestDatabase(t)
	users := []User{{Username: "account", AffCode: "account"}, {Username: "other", AffCode: "other"}}
	require.NoError(t, db.Create(&users).Error)
	token := Token{UserId: users[0].Id, Key: "account-lifecycle", Name: "before", Status: common.TokenStatusEnabled}
	require.NoError(t, InsertOrganizationToken(&token))
	scope := OrganizationTokenScope{UserID: users[0].Id}
	token.Name = "after"
	require.NoError(t, UpdateOrganizationToken(scope, &token, false))
	saved, err := GetOrganizationToken(scope, token.Id)
	require.NoError(t, err)
	assert.Equal(t, "after", saved.Name)
	assert.Zero(t, saved.OrgId)
	_, err = DeleteOrganizationTokens(OrganizationTokenScope{UserID: users[1].Id}, []int{token.Id})
	require.Error(t, err)
	count, err := DeleteOrganizationTokens(scope, []int{token.Id})
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
	var organizations, memberships, audits int64
	require.NoError(t, db.Model(&Organization{}).Count(&organizations).Error)
	require.NoError(t, db.Model(&OrganizationMember{}).Count(&memberships).Error)
	require.NoError(t, db.Model(&OrganizationAudit{}).Count(&audits).Error)
	assert.Zero(t, organizations)
	assert.Zero(t, memberships)
	assert.Zero(t, audits)
}

func TestAccountSubscriptionCannotSpendTeamAllowance(t *testing.T) {
	db := organizationTestDatabase(t)
	require.NoError(t, db.Migrator().DropTable(&SubscriptionPreConsumeRecord{}))
	require.NoError(t, db.AutoMigrate(&SubscriptionPreConsumeRecord{}))
	user := User{Username: "buyer", AffCode: "buyer", Quota: 2000000}
	require.NoError(t, db.Create(&user).Error)
	team, err := CreateTeamOrganization(user.Id, "Team", "subscription-team")
	require.NoError(t, err)
	plan := SubscriptionPlan{Title: "Both", Enabled: true, Audience: "both", PriceAmount: 1,
		DurationUnit: SubscriptionDurationMonth, DurationValue: 1, TotalAmount: 1000, MaxPurchasePerUser: 1}
	require.NoError(t, db.Create(&plan).Error)
	teamSub, err := CreateOrganizationSubscriptionFromPlanTx(db, team.Id, user.Id, &plan, "admin")
	require.NoError(t, err)
	require.NoError(t, PurchaseSubscriptionWithBalance(user.Id, plan.Id))
	var personal UserSubscription
	require.NoError(t, (OrganizationResourceScope{UserID: user.Id}).Apply(db).First(&personal).Error)
	assert.NotEmpty(t, personal.PlanSnapshot)
	count, err := CountUserSubscriptionsByPlan(user.Id, plan.Id)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
	require.Error(t, PurchaseSubscriptionWithBalance(user.Id, plan.Id), "account purchase cap still applies")
	require.NoError(t, db.Delete(&plan).Error)
	receipt, err := PreConsumeUserSubscription("account-request", user.Id, "", 0, 25)
	require.NoError(t, err, "purchased snapshot survives catalog removal")
	assert.Equal(t, personal.Id, receipt.UserSubscriptionId)
	require.NoError(t, db.First(teamSub, teamSub.Id).Error)
	assert.Zero(t, teamSub.AmountUsed)
	require.NoError(t, RefundSubscriptionPreConsume("account-request"))
	require.NoError(t, RefundSubscriptionPreConsume("account-request"))
	require.NoError(t, db.First(&personal, personal.Id).Error)
	assert.Zero(t, personal.AmountUsed)
	require.NoError(t, db.Model(&personal).Update("status", "cancelled").Error)
	active, err := HasActiveUserSubscription(user.Id)
	require.NoError(t, err)
	assert.False(t, active, "a team subscription is not a personal subscription")
}
