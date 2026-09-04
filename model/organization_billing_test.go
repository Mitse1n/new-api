package model

import (
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func organizationBillingFixture(t *testing.T) (*gorm.DB, *Organization, []User) {
	t.Helper()
	db := organizationTestDatabase(t)
	users := []User{{Username: "owner", AffCode: "owner", Email: "owner@example.test", Quota: 999}, {Username: "member", AffCode: "member", Email: "member@example.test", Quota: 888}}
	require.NoError(t, db.Create(&users).Error)
	org, err := CreateTeamOrganization(users[0].Id, "Team", "billing-team")
	require.NoError(t, err)
	require.NoError(t, db.Model(org).Update("quota", 1000).Error)
	require.NoError(t, db.Create(&OrganizationMember{OrgId: org.Id, UserId: users[1].Id, Role: OrgRoleMember, Status: OrganizationActive, SpendLimit: 200}).Error)
	return db, org, users
}

func TestOrganizationChargeCapPrecheckNeverDebitsAnotherWallet(t *testing.T) {
	db, org, users := organizationBillingFixture(t)
	receipt, err := ReserveOrganizationCharge(org.Id, users[1].Id, 0, "first", 150)
	require.NoError(t, err)
	repeated, err := ReserveOrganizationCharge(org.Id, users[1].Id, 0, "first", 150)
	require.NoError(t, err)
	assert.Equal(t, receipt.Id, repeated.Id)
	_, err = ReserveOrganizationCharge(org.Id, users[1].Id, 0, "over-cap", 51)
	assert.ErrorIs(t, err, ErrMemberSpendLimit)
	require.NoError(t, db.First(org, org.Id).Error)
	assert.Equal(t, int64(850), org.Quota)
	require.NoError(t, FinalizeOrganizationCharge(org.Id, "first", 120, false))
	require.NoError(t, FinalizeOrganizationCharge(org.Id, "first", 120, false))
	_, err = ReserveOrganizationCharge(org.Id, users[1].Id, 0, "remaining", 80)
	require.NoError(t, err)
	require.NoError(t, FinalizeOrganizationCharge(org.Id, "remaining", 0, true))
	require.NoError(t, FinalizeOrganizationCharge(org.Id, "remaining", 0, true))
	require.NoError(t, db.First(org, org.Id).Error)
	assert.Equal(t, int64(880), org.Quota)
	assert.Equal(t, int64(120), org.UsedQuota)
	require.NoError(t, db.Order("id").Find(&users).Error)
	assert.Equal(t, 999, users[0].Quota)
	assert.Equal(t, 888, users[1].Quota)
	assert.Error(t, FinalizeOrganizationCharge(org.Id+1, "first", 0, true))
	usage, err := GetOrganizationBudgetUsage(org.Id, org.BudgetPeriodStart)
	require.NoError(t, err)
	require.Len(t, usage, 1)
	assert.Equal(t, int64(120), usage[0].Used)
	assert.Zero(t, usage[0].Reserved)
}

func TestOrganizationReservationsSerializeSharedMemberCap(t *testing.T) {
	db, org, users := organizationBillingFixture(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	// SQLite serializes writes. MySQL/PostgreSQL exercise actual row locking.
	if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		sqlDB.SetMaxOpenConns(1)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, id := range []string{"concurrent-one", "concurrent-two"} {
		wg.Add(1)
		go func(requestID string) {
			defer wg.Done()
			<-start
			_, err := ReserveOrganizationCharge(org.Id, users[1].Id, 0, requestID, 150)
			results <- err
		}(id)
	}
	close(start)
	wg.Wait()
	close(results)
	successes, denials := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else {
			assert.ErrorIs(t, err, ErrMemberSpendLimit)
			denials++
		}
	}
	assert.Equal(t, 1, successes)
	assert.Equal(t, 1, denials)
	require.NoError(t, db.First(org, org.Id).Error)
	assert.Equal(t, int64(850), org.Quota)
}

func TestOrganizationSubscriptionSnapshotAndRefundAcrossReset(t *testing.T) {
	db, org, users := organizationBillingFixture(t)
	no := false
	plan := SubscriptionPlan{Title: "Team plan", Enabled: true, Audience: "org", DurationUnit: SubscriptionDurationMonth, DurationValue: 1, TotalAmount: 300, QuotaResetPeriod: SubscriptionResetDaily, MaxMembers: 2, AllowWalletOverflow: &no}
	require.NoError(t, db.Create(&plan).Error)
	var sub *UserSubscription
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		var err error
		sub, err = CreateOrganizationSubscriptionFromPlanTx(tx, org.Id, users[0].Id, &plan, "order")
		return err
	}))
	receipt, err := ReserveOrganizationCharge(org.Id, users[1].Id, 0, "before-reset", 100)
	require.NoError(t, err)
	assert.Equal(t, sub.Id, receipt.SubscriptionId)
	// Editing/removing the catalog cannot alter purchased terms or break settlement.
	require.NoError(t, db.Delete(&plan).Error)
	snapshot, err := GetOrganizationSubscriptionPlan(db, sub)
	require.NoError(t, err)
	assert.Equal(t, 2, snapshot.MaxMembers)
	require.NoError(t, db.Model(sub).Updates(map[string]interface{}{"last_reset_time": sub.LastResetTime + 86400, "amount_used": 50}).Error)
	require.NoError(t, FinalizeOrganizationCharge(org.Id, "before-reset", 0, true))
	require.NoError(t, db.First(sub, sub.Id).Error)
	assert.Equal(t, int64(50), sub.AmountUsed, "refund of expired allowance must not credit the next period")
	require.NoError(t, db.First(org, org.Id).Error)
	assert.Equal(t, int64(1000), org.Quota)
}

func TestOrganizationTaskRefundIsAtomicAcrossPollRetries(t *testing.T) {
	db, org, users := organizationBillingFixture(t)
	token := Token{OrgId: org.Id, UserId: users[1].Id, Key: "task-key", RemainQuota: 900, UsedQuota: 100}
	require.NoError(t, db.Create(&token).Error)
	_, err := ReserveOrganizationCharge(org.Id, users[1].Id, token.Id, "task-request", 100)
	require.NoError(t, err)
	require.NoError(t, FinalizeOrganizationCharge(org.Id, "task-request", 100, false))
	task := Task{OrgId: org.Id, UserId: users[1].Id, TaskID: "task-test", Quota: 100, PrivateData: TaskPrivateData{TokenId: token.Id, BillingRequestId: "task-request"}}
	require.NoError(t, db.Create(&task).Error)
	stale := task
	delta, err := SettleOrganizationTaskQuota(&task, 0)
	require.NoError(t, err)
	assert.Equal(t, -100, delta)
	delta, err = SettleOrganizationTaskQuota(&stale, 0)
	require.NoError(t, err)
	assert.Zero(t, delta)
	require.NoError(t, db.First(org, org.Id).Error)
	require.NoError(t, db.First(&token, token.Id).Error)
	assert.Equal(t, int64(1000), org.Quota)
	assert.Zero(t, org.UsedQuota)
	assert.Equal(t, 1000, token.RemainQuota)
	assert.Zero(t, token.UsedQuota)
}

func TestOrganizationLifecycleRequiresAcceptedTransferAndSettledFunds(t *testing.T) {
	db, org, users := organizationBillingFixture(t)
	require.NoError(t, RequestOrganizationTransfer(org.Id, users[0].Id, users[1].Id))
	require.NoError(t, db.First(org, org.Id).Error)
	assert.Equal(t, users[0].Id, org.OwnerId)
	assert.ErrorIs(t, AcceptOrganizationTransfer(org.Id, users[0].Id), ErrOrganizationOwner)
	require.NoError(t, AcceptOrganizationTransfer(org.Id, users[1].Id))
	assert.ErrorIs(t, ChangeOrganizationStatus(org.Id, users[0].Id, OrganizationDisabled, ""), ErrOrganizationAccess)
	require.NoError(t, ChangeOrganizationStatus(org.Id, users[1].Id, OrganizationDisabled, ""))
	_, _, err := GetOrganizationMembership(org.Id, users[1].Id)
	assert.ErrorIs(t, err, ErrOrganizationAccess)
	assert.ErrorIs(t, ChangeOrganizationStatus(org.Id, users[1].Id, OrganizationDeleting, org.Slug), ErrOrganizationUnsettled)
	require.NoError(t, ChangeOrganizationStatus(org.Id, users[1].Id, OrganizationActive, ""))
	require.NoError(t, db.Model(org).Update("quota", 0).Error)
	subscription := UserSubscription{OrgId: org.Id, UserId: users[1].Id, Status: "active", EndTime: time.Now().Add(time.Hour).Unix()}
	require.NoError(t, db.Create(&subscription).Error)
	assert.ErrorIs(t, ChangeOrganizationStatus(org.Id, users[1].Id, OrganizationDeleting, org.Slug), ErrOrganizationUnsettled)
	require.NoError(t, db.Model(&subscription).Update("status", "expired").Error)
	require.NoError(t, ChangeOrganizationStatus(org.Id, users[1].Id, OrganizationDeleting, org.Slug))
	assert.ErrorIs(t, db.First(&Organization{}, org.Id).Error, gorm.ErrRecordNotFound)
}

func TestOrganizationViolationFeeAfterRefundIsChargedOnce(t *testing.T) {
	db, org, users := organizationBillingFixture(t)
	token := Token{OrgId: org.Id, UserId: users[1].Id, Key: "fee-token", RemainQuota: 500}
	require.NoError(t, db.Create(&token).Error)
	_, err := ReserveOrganizationCharge(org.Id, users[1].Id, token.Id, "failed-relay", 100)
	require.NoError(t, err)
	require.NoError(t, FinalizeOrganizationCharge(org.Id, "failed-relay", 0, true))
	charged, err := ChargeOrganizationViolationFee(org.Id, users[1].Id, token.Id, "failed-relay", 20)
	require.NoError(t, err)
	assert.True(t, charged)
	charged, err = ChargeOrganizationViolationFee(org.Id, users[1].Id, token.Id, "failed-relay", 20)
	require.NoError(t, err)
	assert.False(t, charged)
	require.NoError(t, db.First(org, org.Id).Error)
	assert.Equal(t, int64(980), org.Quota)
	assert.Equal(t, int64(20), org.UsedQuota)
	require.NoError(t, db.First(&token, token.Id).Error)
	assert.Equal(t, 480, token.RemainQuota)
	assert.Equal(t, 20, token.UsedQuota)
}

func TestOrganizationRequestKeyLimitAndWalletCommitTogether(t *testing.T) {
	db, org, users := organizationBillingFixture(t)
	token := Token{OrgId: org.Id, UserId: users[1].Id, Key: "atomic-key", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 300}
	require.NoError(t, db.Create(&token).Error)
	for _, tc := range []struct {
		id     string
		amount int64
		want   error
	}{
		{"key-cap", 301, ErrOrganizationTokenQuota},
		{"member-cap", 201, ErrMemberSpendLimit},
	} {
		_, err := ReserveOrganizationRequest(org.Id, users[1].Id, token.Id, tc.id, tc.amount)
		assert.ErrorIs(t, err, tc.want)
		require.NoError(t, db.First(&token, token.Id).Error)
		require.NoError(t, db.First(org, org.Id).Error)
		assert.Equal(t, 300, token.RemainQuota)
		assert.Zero(t, token.UsedQuota)
		assert.Equal(t, int64(1000), org.Quota)
	}
	require.NoError(t, db.Model(org).Update("quota", 50).Error)
	_, err := ReserveOrganizationRequest(org.Id, users[1].Id, token.Id, "wallet-cap", 100)
	assert.ErrorIs(t, err, ErrOrganizationQuota)
	require.NoError(t, db.First(&token, token.Id).Error)
	assert.Equal(t, 300, token.RemainQuota, "wallet rejection rolls back the Key reservation")
	require.NoError(t, db.Model(org).Update("quota", 1000).Error)
	for i := 0; i < 2; i++ {
		_, err = ReserveOrganizationRequest(org.Id, users[1].Id, token.Id, "atomic-request", 100)
		require.NoError(t, err)
	}
	for i := 0; i < 2; i++ {
		require.NoError(t, FinalizeOrganizationCharge(org.Id, "atomic-request", 75, false))
	}
	require.NoError(t, db.First(&token, token.Id).Error)
	require.NoError(t, db.First(org, org.Id).Error)
	assert.Equal(t, 225, token.RemainQuota)
	assert.Equal(t, 75, token.UsedQuota)
	assert.Equal(t, int64(925), org.Quota)
	_, err = ReserveOrganizationRequest(org.Id, users[1].Id, token.Id, "refund-request", 100)
	require.NoError(t, err)
	for i := 0; i < 2; i++ {
		require.NoError(t, FinalizeOrganizationCharge(org.Id, "refund-request", 0, true))
	}
	require.NoError(t, db.First(&token, token.Id).Error)
	require.NoError(t, db.First(org, org.Id).Error)
	assert.Equal(t, 225, token.RemainQuota)
	assert.Equal(t, 75, token.UsedQuota)
	assert.Equal(t, int64(925), org.Quota)
	task := Task{OrgId: org.Id, UserId: users[1].Id, TaskID: "atomic-task", Quota: 75, PrivateData: TaskPrivateData{TokenId: token.Id, BillingRequestId: "atomic-request"}}
	require.NoError(t, db.Create(&task).Error)
	stale := task
	delta, err := SettleOrganizationTaskQuota(&task, 0)
	require.NoError(t, err)
	assert.Equal(t, -75, delta)
	delta, err = SettleOrganizationTaskQuota(&stale, 0)
	require.NoError(t, err)
	assert.Zero(t, delta)
	require.NoError(t, db.First(&token, token.Id).Error)
	require.NoError(t, db.First(org, org.Id).Error)
	assert.Equal(t, 300, token.RemainQuota)
	assert.Zero(t, token.UsedQuota)
	assert.Equal(t, int64(1000), org.Quota)
	assert.Zero(t, org.UsedQuota)
}
