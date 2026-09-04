package model

import (
	"fmt"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func organizationTestDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	var dialector gorm.Dialector = sqlite.Open(t.TempDir() + "/organizations.db")
	dialect := common.DatabaseTypeSQLite
	if dsn := os.Getenv("TENANCY_TEST_MYSQL_DSN"); dsn != "" {
		dialector = mysql.Open(dsn)
		dialect = common.DatabaseTypeMySQL
	}
	if dsn := os.Getenv("TENANCY_TEST_POSTGRES_DSN"); dsn != "" {
		dialector = postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
		dialect = common.DatabaseTypePostgreSQL
	}
	db, err := gorm.Open(dialector, &gorm.Config{})
	require.NoError(t, err)
	previousDB, previousLogDB := DB, LOG_DB
	previousRedis := common.RedisEnabled
	common.RedisEnabled = false
	previousDialect, previousLogDialect := common.MainDatabaseType(), common.LogDatabaseType()
	DB, LOG_DB = db, db
	common.SetMainDatabaseType(dialect)
	common.SetLogDatabaseType(dialect)
	initCol()
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLogDB
		common.RedisEnabled = previousRedis
		common.SetMainDatabaseType(previousDialect)
		common.SetLogDatabaseType(previousLogDialect)
		initCol()
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})
	resources := []interface{}{&Organization{}, &OrganizationMember{}, &OrganizationInvite{}, &OrganizationTransfer{}, &OrganizationAudit{}, &OrganizationCharge{}, &OrganizationNotification{}, &User{}, &Token{}, &Log{}, &TopUp{}, &SubscriptionPlan{}, &UserSubscription{}, &SubscriptionOrder{}, &Task{}, &Midjourney{}, &QuotaData{}}
	// External DSNs must point to disposable, isolated test databases.
	for _, resource := range resources {
		require.NoError(t, db.Migrator().DropTable(resource))
	}
	require.NoError(t, db.AutoMigrate(resources...))
	return db
}

func TestOrganizationMigrationPreservesBalancesAndResourceOwners(t *testing.T) {
	db := organizationTestDatabase(t)
	users := []User{{Username: "alice", DisplayName: "Alice", Email: "alice@example.test", AffCode: "alice", Quota: 123456789012, UsedQuota: 123, Group: "premium"}, {Username: "bob", Email: "bob@example.test", AffCode: "bob", Quota: 200, Group: "default"}}
	require.NoError(t, db.Create(&users).Error)
	token := Token{UserId: users[0].Id, Key: "migration-secret", Name: "existing"}
	require.NoError(t, db.Create(&token).Error)
	log := Log{UserId: users[0].Id, Quota: 123, Type: LogTypeConsume}
	require.NoError(t, db.Create(&log).Error)
	require.NoError(t, MigratePersonalOrganizations(db))
	alice, err := GetPersonalOrganization(users[0].Id)
	require.NoError(t, err)
	assert.Equal(t, int64(123456789012), alice.Quota)
	assert.Equal(t, "premium", alice.Group)
	require.NoError(t, db.Model(alice).Update("quota", 42).Error)
	for i := 0; i < 2; i++ {
		require.NoError(t, MigratePersonalOrganizations(db))
	}
	require.NoError(t, db.First(&token, token.Id).Error)
	require.NoError(t, db.First(&log, log.Id).Error)
	assert.Equal(t, alice.Id, token.OrgId)
	assert.Equal(t, alice.Id, log.OrgId)
	alice, err = GetPersonalOrganization(users[0].Id)
	require.NoError(t, err)
	assert.Equal(t, int64(42), alice.Quota, "migration must not copy a legacy balance again")
	var count int64
	require.NoError(t, db.Model(&Organization{}).Count(&count).Error)
	assert.Equal(t, int64(2), count)
	require.NoError(t, db.Model(&OrganizationMember{}).Count(&count).Error)
	assert.Equal(t, int64(2), count)
	duplicate := Organization{Slug: "another-personal", PersonalUserId: &users[0].Id}
	assert.Error(t, db.Create(&duplicate).Error, "one personal organization is a database invariant")
}

func TestOrganizationInvitationRequiresMatchingIdentityAndPreservesAssets(t *testing.T) {
	db := organizationTestDatabase(t)
	users := []User{{Username: "owner", Email: "owner@example.test", AffCode: "owner"}, {Username: "member", Email: "member@example.test", AffCode: "member"}, {Username: "outsider", Email: "outsider@example.test", AffCode: "outsider"}}
	require.NoError(t, db.Create(&users).Error)
	org, err := CreateTeamOrganization(users[0].Id, "Team", "team")
	require.NoError(t, err)
	_, secret, err := CreateOrganizationInvite(org.Id, users[0].Id, "member@example.test", OrgRoleMember)
	require.NoError(t, err)
	_, err = AcceptOrganizationInvite(users[2].Id, secret)
	assert.ErrorIs(t, err, ErrOrganizationInvite)
	id, err := AcceptOrganizationInvite(users[1].Id, secret)
	require.NoError(t, err)
	assert.Equal(t, org.Id, id)
	_, err = AcceptOrganizationInvite(users[1].Id, secret)
	require.NoError(t, err, "acceptance retry is idempotent")
	_, _, err = CreateOrganizationInvite(org.Id, users[1].Id, "outsider@example.test", OrgRoleAdmin)
	assert.ErrorIs(t, err, ErrOrganizationAccess)
	key := Token{OrgId: org.Id, UserId: users[1].Id, Key: "retained-key"}
	require.NoError(t, db.Create(&key).Error)
	require.NoError(t, UpdateOrganizationMember(org.Id, users[0].Id, users[1].Id, OrgRoleMember, OrganizationDeleting, 100))
	_, _, err = GetOrganizationMembership(org.Id, users[1].Id)
	assert.ErrorIs(t, err, ErrOrganizationAccess)
	require.NoError(t, db.First(&key, key.Id).Error)
	assert.Equal(t, org.Id, key.OrgId)
	assert.ErrorIs(t, UpdateOrganizationMember(org.Id, users[0].Id, users[0].Id, OrgRoleAdmin, OrganizationDeleting, 0), ErrOrganizationOwner)
	for _, scopeID := range []int{0, -1, org.Id + 1} {
		t.Run(fmt.Sprint(scopeID), func(t *testing.T) {
			var tokens []Token
			require.NoError(t, db.Scopes(OrgScope(scopeID)).Find(&tokens).Error)
			assert.Empty(t, tokens)
		})
	}
}
