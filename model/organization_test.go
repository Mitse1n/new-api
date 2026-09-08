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

func TestAccountAndTeamResourceIsolation(t *testing.T) {
	db := organizationTestDatabase(t)
	users := []User{{Username: "alice", AffCode: "alice", Quota: 500}, {Username: "bob", AffCode: "bob"}}
	require.NoError(t, db.Create(&users).Error)
	org, err := CreateTeamOrganization(users[0].Id, "Team", "team")
	require.NoError(t, err)
	keys := []Token{
		{UserId: users[0].Id, Key: "alice-personal", Name: "alice"},
		{UserId: users[1].Id, Key: "bob-personal", Name: "bob"},
		{OrgId: org.Id, UserId: users[0].Id, Key: "team-key", Name: "team"},
	}
	require.NoError(t, db.Create(&keys).Error)
	for _, test := range []struct {
		name  string
		scope OrganizationResourceScope
		names []string
	}{
		{"account", OrganizationResourceScope{UserID: users[0].Id}, []string{"alice"}},
		{"account cannot widen", OrganizationResourceScope{UserID: users[0].Id, AllMembers: true}, []string{"alice"}},
		{"team", OrganizationResourceScope{OrgID: org.Id, UserID: users[0].Id}, []string{"team"}},
		{"missing user", OrganizationResourceScope{AllMembers: true}, []string{}},
		{"negative organization", OrganizationResourceScope{OrgID: -1, UserID: users[0].Id}, []string{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var names []string
			require.NoError(t, test.scope.Apply(db.Model(&Token{})).Pluck("name", &names).Error)
			assert.ElementsMatch(t, test.names, names)
		})
	}
	var count int64
	require.NoError(t, db.Model(&Organization{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestOrganizationInvitationRequiresMatchingIdentityAndPreservesAssets(t *testing.T) {
	db := organizationTestDatabase(t)
	users := []User{{Username: "owner", Email: "owner@example.test", AffCode: "owner"}, {Username: "member", AffCode: "member"}, {Username: "outsider", Email: "outsider@example.test", AffCode: "outsider"}}
	require.NoError(t, db.Create(&users).Error)
	org, err := CreateTeamOrganization(users[0].Id, "Team", "team")
	require.NoError(t, err)
	invite, err := CreateOrganizationInvite(org.Id, users[0].Id, "member", OrgRoleMember)
	require.NoError(t, err)
	_, err = AcceptOrganizationInvite(users[2].Id, invite.Id)
	assert.ErrorIs(t, err, ErrOrganizationInvite)
	id, err := AcceptOrganizationInvite(users[1].Id, invite.Id)
	require.NoError(t, err)
	assert.Equal(t, org.Id, id)
	_, err = AcceptOrganizationInvite(users[1].Id, invite.Id)
	require.NoError(t, err, "acceptance retry is idempotent")
	_, err = CreateOrganizationInvite(org.Id, users[1].Id, "outsider", OrgRoleAdmin)
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
