package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestOrganizationPaymentsUsePersistedOwnerAndImmutableTerms(t *testing.T) {
	db, org, users := organizationBillingFixture(t)
	other, err := CreateTeamOrganization(users[0].Id, "Other team", "other-payments")
	require.NoError(t, err)
	plan := SubscriptionPlan{Title: "Purchased terms", Enabled: true, Audience: "org", PriceAmount: 10, DurationUnit: SubscriptionDurationMonth, DurationValue: 1, TotalAmount: 500, UpgradeGroup: "premium", MaxMembers: 3}
	require.NoError(t, db.Create(&plan).Error)
	for _, provider := range []string{PaymentProviderEpay, PaymentMethodStripe, PaymentMethodCreem, PaymentMethodWaffo, PaymentMethodWaffoPancake} {
		t.Run(provider, func(t *testing.T) {
			require.NoError(t, db.Model(&plan).Updates(map[string]interface{}{"total_amount": 500, "enabled": true}).Error)
			order := SubscriptionOrder{OrgId: org.Id, UserId: users[0].Id, PlanId: plan.Id, Money: 10, TradeNo: "org-payment-" + provider, PaymentMethod: provider, PaymentProvider: provider, Status: common.TopUpStatusPending}
			require.NoError(t, order.Insert())
			require.NoError(t, db.Model(&plan).Updates(map[string]interface{}{"total_amount": 1, "enabled": false}).Error)
			assert.ErrorIs(t, CompleteSubscriptionOrder(order.TradeNo, "", "wrong-provider", ""), ErrPaymentMethodMismatch)
			require.NoError(t, CompleteSubscriptionOrder(order.TradeNo, `{"org_id":9999}`, provider, ""))
			require.NoError(t, CompleteSubscriptionOrder(order.TradeNo, `{"org_id":9999}`, provider, ""))
			order.Status = common.TopUpStatusFailed
			require.NoError(t, order.Update())
			require.NoError(t, db.First(&order, order.Id).Error)
			assert.Equal(t, common.TopUpStatusSuccess, order.Status)
			var sub UserSubscription
			require.NoError(t, db.Where("org_id = ?", org.Id).Order("id DESC").First(&sub).Error)
			assert.Equal(t, int64(500), sub.AmountTotal)
			assert.Equal(t, org.Id, sub.OrgId)
		})
	}
	var count int64
	require.NoError(t, db.Model(&UserSubscription{}).Scopes(OrgScope(org.Id)).Count(&count).Error)
	assert.Equal(t, int64(5), count)
	require.NoError(t, db.Model(&UserSubscription{}).Scopes(OrgScope(other.Id)).Count(&count).Error)
	assert.Zero(t, count)
	require.NoError(t, db.First(other, other.Id).Error)
	assert.Equal(t, "default", other.Group)
	require.NoError(t, db.Model(&Log{}).Where("org_id = ? AND type = ?", org.Id, LogTypeTopup).Count(&count).Error)
	assert.Equal(t, int64(5), count)
}

func TestOrganizationExpiryCannotChangeAnotherOrganizationsTier(t *testing.T) {
	db, org, users := organizationBillingFixture(t)
	other, err := CreateTeamOrganization(users[0].Id, "Other tier", "other-tier")
	require.NoError(t, err)
	plan := SubscriptionPlan{Title: "Tier", Enabled: true, Audience: "org", DurationUnit: SubscriptionDurationMonth, DurationValue: 1, TotalAmount: 500, UpgradeGroup: "premium"}
	require.NoError(t, db.Create(&plan).Error)
	var first, second, unrelated *UserSubscription
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		var err error
		first, err = CreateOrganizationSubscriptionFromPlanTx(tx, org.Id, users[0].Id, &plan, "admin")
		if err != nil {
			return err
		}
		plan.UpgradeGroup = "enterprise"
		second, err = CreateOrganizationSubscriptionFromPlanTx(tx, org.Id, users[0].Id, &plan, "admin")
		if err != nil {
			return err
		}
		plan.UpgradeGroup = "unrelated"
		unrelated, err = CreateOrganizationSubscriptionFromPlanTx(tx, other.Id, users[0].Id, &plan, "admin")
		return err
	}))
	require.NoError(t, db.Model(second).Update("end_time", common.GetTimestamp()-1).Error)
	n, err := ExpireDueSubscriptions(100)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	require.NoError(t, db.First(org, org.Id).Error)
	assert.Equal(t, "premium", org.Group)
	require.NoError(t, db.First(other, other.Id).Error)
	assert.Equal(t, "unrelated", other.Group)
	require.NoError(t, db.Model(first).Update("end_time", common.GetTimestamp()-1).Error)
	_, err = ExpireDueSubscriptions(100)
	require.NoError(t, err)
	require.NoError(t, db.First(org, org.Id).Error)
	assert.Equal(t, "default", org.Group)
	require.NoError(t, db.First(unrelated, unrelated.Id).Error)
	assert.Equal(t, "active", unrelated.Status)
	require.NoError(t, db.First(&users[0], users[0].Id).Error)
	assert.Equal(t, "default", users[0].Group)
}

func TestOrganizationBudgetAlertsDeduplicateSettlements(t *testing.T) {
	db, org, users := organizationBillingFixture(t)
	require.NoError(t, db.Model(&User{}).Where("id IN ?", []int{users[0].Id, users[1].Id}).Update("status", common.UserStatusEnabled).Error)
	settings := OrganizationSettings{BudgetLimit: 100, AlertPercent: 80, AlertEmail: "finance@example.test", Webhook: "https://alerts.example.test/budget"}
	payload, err := common.Marshal(settings)
	require.NoError(t, err)
	require.NoError(t, db.Model(org).Update("settings", string(payload)).Error)
	_, err = ReserveOrganizationCharge(org.Id, users[1].Id, 0, "alert-first", 80)
	require.NoError(t, err)
	require.NoError(t, FinalizeOrganizationCharge(org.Id, "alert-first", 80, false))
	require.NoError(t, FinalizeOrganizationCharge(org.Id, "alert-first", 80, false))
	_, err = ReserveOrganizationCharge(org.Id, users[1].Id, 0, "alert-next", 10)
	require.NoError(t, err)
	require.NoError(t, FinalizeOrganizationCharge(org.Id, "alert-next", 10, false))
	var notifications []OrganizationNotification
	require.NoError(t, db.Scopes(OrgScope(org.Id)).Find(&notifications).Error)
	require.Len(t, notifications, 3)
	destinations := make([]string, 0, len(notifications))
	for _, n := range notifications {
		destinations = append(destinations, n.Destination)
		assert.Equal(t, "pending", n.Status)
	}
	assert.ElementsMatch(t, []string{users[0].Email, "finance@example.test", "https://alerts.example.test/budget"}, destinations)
}

func TestSubscriptionOrderCompletionEnforcesPurchaseLimit(t *testing.T) {
	for _, team := range []bool{false, true} {
		t.Run(fmt.Sprint("team=", team), func(t *testing.T) {
			db, org, users := organizationBillingFixture(t)
			plan := SubscriptionPlan{Title: "Limited", Enabled: true, Audience: "both", PriceAmount: 10, MaxPurchasePerUser: 1, DurationUnit: SubscriptionDurationMonth, DurationValue: 1, TotalAmount: 500}
			require.NoError(t, db.Create(&plan).Error)
			orgID := 0
			if team {
				orgID = org.Id
			}
			orders := []SubscriptionOrder{
				{TradeNo: "first", CreateTime: common.GetTimestamp() - 25*3600},
				{TradeNo: "second"},
				{TradeNo: "expired"},
				{TradeNo: "failed"},
			}
			for i := range orders {
				order := &orders[i]
				order.OrgId, order.UserId, order.PlanId = orgID, users[0].Id, plan.Id
				order.Money, order.PaymentProvider, order.Status = 10, PaymentProviderEpay, common.TopUpStatusPending
				require.NoError(t, order.Insert(), "pending orders do not reserve purchase slots")
			}
			require.NoError(t, ExpireSubscriptionOrder("expired", PaymentProviderEpay))
			orders[3].Status = common.TopUpStatusFailed
			require.NoError(t, orders[3].Update())
			for _, tradeNo := range []string{"expired", "failed"} {
				assert.ErrorIs(t, CompleteSubscriptionOrder(tradeNo, "", PaymentProviderEpay, ""), ErrSubscriptionOrderStatusInvalid)
			}
			require.NoError(t, CompleteSubscriptionOrder("first", "", PaymentProviderEpay, ""))
			require.NoError(t, CompleteSubscriptionOrder("first", "", PaymentProviderEpay, ""), "successful callbacks are idempotent")
			assert.EqualError(t, CompleteSubscriptionOrder("second", "", PaymentProviderEpay, ""), "plan purchase limit reached")
			var subs []UserSubscription
			require.NoError(t, db.Where("org_id = ? AND user_id = ?", orgID, users[0].Id).Find(&subs).Error)
			require.Len(t, subs, 1)
			assert.Equal(t, int64(500), subs[0].AmountTotal)
			require.NoError(t, db.First(&orders[1], orders[1].Id).Error)
			assert.Equal(t, common.TopUpStatusPending, orders[1].Status)
			var topups int64
			require.NoError(t, db.Model(&TopUp{}).Where("org_id = ? AND user_id = ?", orgID, users[0].Id).Count(&topups).Error)
			assert.Equal(t, int64(1), topups)
		})
	}
}

func TestConcurrentSubscriptionOrdersSharePurchaseLimit(t *testing.T) {
	for _, team := range []bool{false, true} {
		t.Run(fmt.Sprint("team=", team), func(t *testing.T) {
			db, org, users := organizationBillingFixture(t)
			if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
				t.Skip("row-lock concurrency is verified on MySQL and PostgreSQL; SQLite issuance is covered by the sequential test")
			}
			plan := SubscriptionPlan{Title: "Limited", Enabled: true, Audience: "both", PriceAmount: 10, MaxPurchasePerUser: 1, DurationUnit: SubscriptionDurationMonth, DurationValue: 1, TotalAmount: 500}
			require.NoError(t, db.Create(&plan).Error)
			orgID := 0
			if team {
				orgID = org.Id
				require.NoError(t, db.Model(&OrganizationMember{}).Where("org_id = ? AND user_id = ?", orgID, users[1].Id).Update("role", OrgRoleAdmin).Error)
			}
			for i, tradeNo := range []string{"concurrent-first", "concurrent-second"} {
				userID := users[0].Id
				if team {
					userID = users[i].Id
				}
				order := SubscriptionOrder{OrgId: orgID, UserId: userID, PlanId: plan.Id, Money: 10, TradeNo: tradeNo, PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusPending}
				require.NoError(t, order.Insert())
				// Released orders have no snapshot and must remain subject to
				// the same serialized purchase limit after upgrade.
				require.NoError(t, db.Model(&order).Update("plan_snapshot", "").Error)
			}
			start := make(chan struct{})
			results := make(chan error, 2)
			for _, tradeNo := range []string{"concurrent-first", "concurrent-second"} {
				go func(tradeNo string) {
					<-start
					results <- CompleteSubscriptionOrder(tradeNo, "", PaymentProviderEpay, "")
				}(tradeNo)
			}
			close(start)
			successes := 0
			for range 2 {
				if err := <-results; err != nil {
					assert.EqualError(t, err, "plan purchase limit reached")
				} else {
					successes++
				}
			}
			assert.Equal(t, 1, successes)
			var subs []UserSubscription
			require.NoError(t, db.Where("org_id = ? AND plan_id = ?", orgID, plan.Id).Find(&subs).Error)
			require.Len(t, subs, 1)
			var completed int64
			require.NoError(t, db.Model(&SubscriptionOrder{}).Where("org_id = ? AND plan_id = ? AND status = ?", orgID, plan.Id, common.TopUpStatusSuccess).Count(&completed).Error)
			assert.Equal(t, int64(1), completed)
		})
	}
}

func TestOrganizationRedemptionLogMatchesCreditedWallet(t *testing.T) {
	db, org, users := organizationBillingFixture(t)
	require.NoError(t, db.Migrator().DropTable(&Redemption{}))
	require.NoError(t, db.AutoMigrate(&Redemption{}))
	for _, orgID := range []int{0, org.Id} {
		code := Redemption{Key: fmt.Sprintf("review-redeem-%d", orgID), Quota: 100, Status: common.RedemptionCodeStatusEnabled}
		require.NoError(t, code.Insert())
		quota, err := Redeem(code.Key, users[0].Id, orgID)
		require.NoError(t, err)
		assert.Equal(t, 100, quota)
		var log Log
		require.NoError(t, db.Where("type = ?", LogTypeTopup).Order("id DESC").First(&log).Error)
		assert.Equal(t, orgID, log.OrgId)
		assert.Equal(t, users[0].Id, log.UserId)
		require.NoError(t, db.First(&users[0], users[0].Id).Error)
		assert.Equal(t, 1099, users[0].Quota)
		require.NoError(t, db.First(org, org.Id).Error)
		if orgID == 0 {
			assert.Equal(t, int64(1000), org.Quota)
		} else {
			assert.Equal(t, int64(1100), org.Quota)
		}
	}
}

func TestOrganizationQuotaAggregationPreservesLegacyPersonalBucket(t *testing.T) {
	db := organizationTestDatabase(t)
	previous := CacheQuotaData
	CacheQuotaData = make(map[string]*QuotaData)
	t.Cleanup(func() { CacheQuotaData = previous })
	legacy := QuotaData{UserID: 7, Username: "alice", ModelName: "model", CreatedAt: 3600, Count: 2, Quota: 20, TokenUsed: 4}
	require.NoError(t, db.Create(&legacy).Error)
	require.NoError(t, db.Model(&legacy).Update("org_id", nil).Error)
	team := QuotaData{OrgId: 19, UserID: 7, Username: "alice", ModelName: "model", CreatedAt: 3600, Count: 3, Quota: 30, TokenUsed: 6}
	require.NoError(t, db.Create(&team).Error)
	for _, orgID := range []int{0, 19} {
		LogQuotaData(QuotaDataLogParams{OrgId: orgID, UserID: 7, Username: "alice", ModelName: "model", CreatedAt: 3601, Quota: 10, TokenUsed: 2})
	}
	SaveQuotaDataCache()
	var rows []QuotaData
	require.NoError(t, db.Order("id").Find(&rows).Error)
	require.Len(t, rows, 2)
	assert.Equal(t, legacy.Id, rows[0].Id)
	assert.Equal(t, 30, rows[0].Quota)
	assert.Equal(t, 3, rows[0].Count)
	assert.Equal(t, 40, rows[1].Quota)
	assert.Equal(t, 4, rows[1].Count)
	// Deployments may already contain both NULL and zero buckets. Increment only
	// one matching bucket, otherwise each new charge is counted twice.
	duplicate := legacy
	duplicate.Id, duplicate.OrgId, duplicate.Quota = 0, 0, 10
	require.NoError(t, db.Create(&duplicate).Error)
	LogQuotaData(QuotaDataLogParams{UserID: 7, Username: "alice", ModelName: "model", CreatedAt: 3601, Quota: 10})
	SaveQuotaDataCache()
	var total int64
	require.NoError(t, db.Model(&QuotaData{}).Scopes((ResourceScope{UserID: 7}).Apply).Select("SUM(quota)").Scan(&total).Error)
	assert.Equal(t, int64(50), total)
}

func TestDeletingOrganizationPreservesPlatformRedemptionCodes(t *testing.T) {
	db, org, _ := organizationBillingFixture(t)
	require.NoError(t, db.AutoMigrate(&Redemption{}, &CasbinRule{}))
	code := Redemption{Key: "global-code", Name: "global", Quota: 100, Status: common.RedemptionCodeStatusEnabled}
	require.NoError(t, db.Create(&code).Error)
	require.NoError(t, db.Model(org).Update("status", OrganizationDeleting).Error)
	require.NoError(t, db.Delete(org).Error)
	require.NoError(t, CleanupDeletedOrganizations())
	require.NoError(t, db.First(&code, code.Id).Error)
	assert.Equal(t, common.RedemptionCodeStatusEnabled, code.Status)
	assert.Equal(t, 100, code.Quota)
	var count int64
	require.NoError(t, db.Unscoped().Model(&Organization{}).Where("id = ?", org.Id).Count(&count).Error)
	assert.Zero(t, count)
}
