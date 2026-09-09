package model

import (
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"testing"
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
			// A timeout can race a later signed successful callback.
			require.NoError(t, ExpireSubscriptionOrder(order.TradeNo, provider))
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

func TestAbandonedSubscriptionCheckoutReleasesPurchaseLimit(t *testing.T) {
	for _, team := range []bool{false, true} {
		t.Run(fmt.Sprint("team=", team), func(t *testing.T) {
			db, org, users := organizationBillingFixture(t)
			plan := SubscriptionPlan{Title: "Limited", Enabled: true, Audience: "both", PriceAmount: 10, MaxPurchasePerUser: 1, DurationUnit: SubscriptionDurationMonth, DurationValue: 1, TotalAmount: 500}
			require.NoError(t, db.Create(&plan).Error)
			orgID := 0
			if team {
				orgID = org.Id
			}
			order := SubscriptionOrder{OrgId: orgID, UserId: users[0].Id, PlanId: plan.Id, Money: 10, TradeNo: "abandoned", PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusPending}
			require.NoError(t, order.Insert())
			validate := func() error {
				if team {
					return ValidateOrganizationPlan(db, org, &plan)
				}
				return ValidateAccountSubscriptionPlan(db, users[0].Id, &plan)
			}
			assert.Error(t, validate(), "a fresh checkout reserves its purchase slot")
			require.NoError(t, db.Model(&order).Update("create_time", common.GetTimestamp()-25*3600).Error)
			assert.NoError(t, validate(), "abandoned checkout must not reserve the slot forever")
			replacement := SubscriptionOrder{OrgId: orgID, UserId: users[0].Id, PlanId: plan.Id, Money: 10, TradeNo: "replacement", PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusPending}
			require.NoError(t, replacement.Insert())
			require.NoError(t, ExpirePendingSubscriptionOrders())
			require.NoError(t, ExpirePendingSubscriptionOrders(), "maintenance is idempotent")
			require.NoError(t, db.First(&order, order.Id).Error)
			assert.Equal(t, common.TopUpStatusExpired, order.Status)
			require.NoError(t, db.First(&replacement, replacement.Id).Error)
			assert.Equal(t, common.TopUpStatusPending, replacement.Status)
			assert.Error(t, validate(), "the replacement reserves a new slot")
			// A local deadline cannot discard money already collected upstream.
			assert.ErrorIs(t, CompleteSubscriptionOrder(order.TradeNo, "", PaymentProviderStripe, ""), ErrPaymentMethodMismatch)
			require.NoError(t, CompleteSubscriptionOrder(order.TradeNo, "", PaymentProviderEpay, ""))
			require.NoError(t, CompleteSubscriptionOrder(order.TradeNo, "", PaymentProviderEpay, ""))
			require.NoError(t, ExpirePendingSubscriptionOrders())
			require.NoError(t, db.First(&order, order.Id).Error)
			assert.Equal(t, common.TopUpStatusSuccess, order.Status)
			var subs []UserSubscription
			require.NoError(t, db.Where("org_id = ? AND user_id = ?", orgID, users[0].Id).Find(&subs).Error)
			require.Len(t, subs, 1)
			assert.Equal(t, int64(500), subs[0].AmountTotal)

		})
	}
}
