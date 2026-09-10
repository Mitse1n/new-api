package service

import (
	"os"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestOrganizationPostConsumeUsesBillingSession(t *testing.T) {
	for _, actual := range []int{70, 100, 130} {
		t.Run(strconv.Itoa(actual), func(t *testing.T) {
			var driver gorm.Dialector = sqlite.Open(t.TempDir() + "/billing.db")
			dialect := common.DatabaseTypeSQLite
			if dsn := os.Getenv("TENANCY_TEST_MYSQL_DSN"); dsn != "" {
				driver = mysql.Open(dsn)
				dialect = common.DatabaseTypeMySQL
			}
			if dsn := os.Getenv("TENANCY_TEST_POSTGRES_DSN"); dsn != "" {
				driver = postgres.Open(dsn)
				dialect = common.DatabaseTypePostgreSQL
			}
			db, err := gorm.Open(driver, &gorm.Config{})
			require.NoError(t, err)
			previousDB, previousRedis, previousDialect := model.DB, common.RedisEnabled, common.MainDatabaseType()
			model.DB, common.RedisEnabled = db, false
			common.SetMainDatabaseType(dialect)
			t.Cleanup(func() {
				model.DB, common.RedisEnabled = previousDB, previousRedis
				common.SetMainDatabaseType(previousDialect)
				sqlDB, err := db.DB()
				require.NoError(t, err)
				require.NoError(t, sqlDB.Close())
			})
			resources := []any{&model.Organization{}, &model.OrganizationMember{}, &model.OrganizationCharge{}, &model.OrganizationAudit{}, &model.OrganizationNotification{}, &model.UserSubscription{}, &model.Token{}}
			for _, resource := range resources {
				require.NoError(t, db.Migrator().DropTable(resource))
			}
			require.NoError(t, db.AutoMigrate(resources...))
			org := model.Organization{Name: "Quota", Slug: "quota", Status: model.OrganizationActive, Quota: 1000}
			require.NoError(t, db.Create(&org).Error)
			member := model.OrganizationMember{OrgId: org.Id, UserId: 1, Role: model.OrgRoleOwner, Status: model.OrganizationActive}
			require.NoError(t, db.Create(&member).Error)
			token := model.Token{OrgId: org.Id, UserId: 1, Key: "settle-once", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 1000}
			require.NoError(t, db.Create(&token).Error)
			info := &relaycommon.RelayInfo{OrgId: org.Id, UserId: 1, TokenId: token.Id, TokenKey: token.Key, RequestId: "settle-once"}
			require.Nil(t, PreConsumeBilling(&gin.Context{}, 100, info))
			for range 2 {
				result, err := postConsumeQuotaWithResult(info, actual-100, 100, false)
				require.NoError(t, err)
				assert.True(t, result.FundingApplied)
				assert.True(t, result.TokenApplied)
			}
			require.NoError(t, db.First(&token, token.Id).Error)
			require.NoError(t, db.First(&org, org.Id).Error)
			assert.Equal(t, 1000-actual, token.RemainQuota)
			assert.Equal(t, actual, token.UsedQuota)
			assert.Equal(t, int64(1000-actual), org.Quota)
			assert.Equal(t, int64(actual), org.UsedQuota)
		})
	}
}

func TestOrganizationFreeSettlementWithoutReservation(t *testing.T) {
	for _, orgID := range []int{0, 1} {
		info := &relaycommon.RelayInfo{OrgId: orgID}
		require.NoError(t, SettleBilling(&gin.Context{}, info, 0))
	}
	info := &relaycommon.RelayInfo{OrgId: 1}
	result, err := postConsumeQuotaWithResult(info, 0, 0, false)
	require.NoError(t, err)
	assert.False(t, result.FundingApplied)
	assert.False(t, result.TokenApplied)
	result, err = postConsumeQuotaWithResult(info, 1, 0, false)
	require.EqualError(t, err, "organization billing session is missing")
	assert.False(t, result.FundingApplied)
	assert.False(t, result.TokenApplied)
}
