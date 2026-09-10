package model

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestScopedTaskReadsSeparateLegacyPersonalAndOrganizationResources(t *testing.T) {
	db := organizationTestDatabase(t)
	personalTask := Task{TaskID: "shared-id", UserId: 7, Platform: "document"}
	personalMJ := Midjourney{MjId: "shared-id", UserId: 7}
	require.NoError(t, db.Create(&personalTask).Error)
	require.NoError(t, db.Create(&personalMJ).Error)
	require.NoError(t, db.Model(&personalTask).Update("org_id", nil).Error)
	require.NoError(t, db.Model(&personalMJ).Update("org_id", nil).Error)
	for _, owner := range []struct{ user, org int }{{7, 19}, {7, 23}, {8, 19}} {
		require.NoError(t, db.Create(&Task{TaskID: "shared-id", UserId: owner.user, OrgId: owner.org, Platform: "document"}).Error)
		require.NoError(t, db.Create(&Midjourney{MjId: "shared-id", UserId: owner.user, OrgId: owner.org}).Error)
	}
	for _, org := range []int{0, 19, 23} {
		scope := ResourceScope{UserID: 7, OrgID: org}
		tasks := TaskGetAllUserTask(scope, 0, 10, SyncTaskQueryParams{})
		require.Len(t, tasks, 1)
		assert.Equal(t, org, tasks[0].OrgId)
		assert.Equal(t, int64(1), TaskCountAllUserTask(scope, SyncTaskQueryParams{}))
		tasks, err := GetByTaskIdsForPlatforms(scope, []constant.TaskPlatform{"document"}, []string{"shared-id"})
		require.NoError(t, err)
		require.Len(t, tasks, 1)
		assert.Equal(t, org, tasks[0].OrgId)
		mj := GetByMJId(scope, "shared-id")
		require.NotNil(t, mj)
		assert.Equal(t, org, mj.OrgId)
		mjs := GetByMJIds(scope, []string{"shared-id"})
		require.Len(t, mjs, 1)
		assert.Equal(t, org, mjs[0].OrgId)
		mjs = GetAllUserTask(scope, 0, 10, TaskQueryParams{})
		require.Len(t, mjs, 1)
		assert.Equal(t, org, mjs[0].OrgId)
		assert.Equal(t, int64(1), CountAllUserTask(scope, TaskQueryParams{}))
	}
}

func TestPersonalSubscriptionReadsIncludeLegacyNullAndExcludeOrganizations(t *testing.T) {
	db := organizationTestDatabase(t)
	now := common.GetTimestamp()
	for _, owner := range []struct{ user, org int }{{7, 0}, {7, 19}, {8, 0}} {
		sub := UserSubscription{UserId: owner.user, OrgId: owner.org, PlanId: 1, Status: "active", StartTime: now - 1, EndTime: now + 3600, AllowWalletOverflow: owner.org == 0}
		require.NoError(t, db.Create(&sub).Error)
		if owner.org == 0 {
			require.NoError(t, db.Model(&sub).Update("org_id", nil).Error)
		}
	}
	subs, err := GetAllUserSubscriptions(7)
	require.NoError(t, err)
	require.Len(t, subs, 1)
	assert.Zero(t, subs[0].Subscription.OrgId)
	subs, err = GetAllActiveUserSubscriptions(7)
	require.NoError(t, err)
	require.Len(t, subs, 1)
	count, err := CountUserSubscriptionsByPlan(7, 1)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
	active, err := HasActiveUserSubscription(7)
	require.NoError(t, err)
	assert.True(t, active)
	overflow, err := UserActiveSubscriptionsAllowWalletOverflow(7)
	require.NoError(t, err)
	assert.True(t, overflow, "an organization's strict subscription must not block the personal wallet")
}
