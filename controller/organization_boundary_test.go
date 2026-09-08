package controller

import (
	"bytes"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// External DSNs must name disposable databases created only for this test.
func TestOrganizationPublicAPIBoundary(t *testing.T) {
	var dialector gorm.Dialector = sqlite.Open(t.TempDir() + "/boundary.db")
	dialect := common.DatabaseTypeSQLite
	if dsn := os.Getenv("ORGANIZATION_API_TEST_MYSQL_DSN"); dsn != "" {
		dialector, dialect = mysql.Open(dsn), common.DatabaseTypeMySQL
	}
	if dsn := os.Getenv("ORGANIZATION_API_TEST_POSTGRES_DSN"); dsn != "" {
		dialector, dialect = postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true}), common.DatabaseTypePostgreSQL
	}
	db, err := gorm.Open(dialector, &gorm.Config{})
	require.NoError(t, err)
	oldDB, oldLog, oldRedis := model.DB, model.LOG_DB, common.RedisEnabled
	oldDialect, oldLogDialect := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB, model.LOG_DB, common.RedisEnabled = db, db, false
	common.SetMainDatabaseType(dialect)
	common.SetLogDatabaseType(dialect)
	t.Cleanup(func() {
		model.DB, model.LOG_DB, common.RedisEnabled = oldDB, oldLog, oldRedis
		common.SetMainDatabaseType(oldDialect)
		common.SetLogDatabaseType(oldLogDialect)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})
	resources := []any{&model.User{}, &model.Organization{}, &model.OrganizationMember{}, &model.OrganizationAudit{}, &model.OrganizationTransfer{}, &model.OrganizationCharge{}, &model.Token{}, &model.Log{}, &model.TopUp{}, &model.UserSubscription{}, &model.SubscriptionOrder{}, &model.CasbinRule{}, &model.AuthzRole{}}
	for _, resource := range resources {
		require.NoError(t, db.Migrator().DropTable(resource))
	}
	require.NoError(t, db.AutoMigrate(resources...))
	require.NoError(t, authz.Init(db))
	var version string
	versionQuery := "SELECT version()"
	if dialect == common.DatabaseTypeSQLite {
		versionQuery = "SELECT sqlite_version()"
	}
	require.NoError(t, db.Raw(versionQuery).Scan(&version).Error)
	t.Logf("database: %s", version)
	owner := model.User{Username: "owner", DisplayName: "Owner Name", Password: "private-password", Email: "private@example.test", AffCode: "owner", Status: 1, Quota: 12345}
	require.NoError(t, db.Create(&owner).Error)
	team, err := model.CreateTeamOrganization(owner.Id, "Design team", "design")
	require.NoError(t, err)
	key := model.Token{UserId: owner.Id, Name: "own-account-key", Key: "private-token-key"}
	require.NoError(t, db.Create(&key).Error)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("id", owner.Id); c.Set("role", common.RoleRootUser); c.Next() })
	r.GET("/organizations", ListOrganizations)
	r.GET("/platform/organizations", PlatformListOrganizations)
	r.GET("/platform/organizations/:org_id/resources/:resource", PlatformOrganizationResources)
	r.PUT("/platform/organizations/:org_id/status", PlatformChangeOrganizationStatus)
	r.GET("/organizations/:org_id/deletion-impact", GetOrganizationDeletionImpact)
	r.PUT("/organizations/:org_id/status", ChangeOrganizationStatus)
	shared := r.Group("/shared", middleware.TeamOrganizationContext())
	shared.GET("/tokens", GetAllTokens)
	org := r.Group("/org", middleware.TeamOrganizationContext(), middleware.RequireTeamOrganization())
	org.GET("/context", GetOrganizationContext)
	org.GET("/members", GetOrganizationMembers)

	request := func(method, path, header, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		if header != "" {
			req.Header.Set("X-Org-Id", header)
		}
		result := httptest.NewRecorder()
		r.ServeHTTP(result, req)
		return result
	}
	t.Run("platform pagination search and owners expose only teams", func(t *testing.T) {
		for _, path := range []string{"/platform/organizations?size=1", "/platform/organizations?keyword=design"} {
			result := request("GET", path, "", "")
			require.Equal(t, 200, result.Code)
			var body struct {
				Data struct {
					Total int
					Items []struct {
						ID               int
						OwnerUsername    string `json:"owner_username"`
						OwnerDisplayName string `json:"owner_display_name"`
					}
				}
			}
			require.NoError(t, common.Unmarshal(result.Body.Bytes(), &body))
			require.Equal(t, 1, body.Data.Total)
			require.Len(t, body.Data.Items, 1)
			assert.Equal(t, team.Id, body.Data.Items[0].ID)
			assert.Equal(t, "owner", body.Data.Items[0].OwnerUsername)
			assert.Equal(t, "Owner Name", body.Data.Items[0].OwnerDisplayName)
			assert.NotContains(t, result.Body.String(), "private-password")
			assert.NotContains(t, result.Body.String(), "private@example.test")
		}
		result := request("GET", "/organizations", "", "")
		assert.Contains(t, result.Body.String(), "Design team")
	})
	t.Run("an account without a team keeps its own keys and never leaks the secret", func(t *testing.T) {
		result := request("GET", "/shared/tokens", "", "")
		require.Equal(t, 200, result.Code)
		assert.Contains(t, result.Body.String(), "own-account-key")
		assert.NotContains(t, result.Body.String(), key.Key)
		var stored model.Token
		require.NoError(t, db.First(&stored, key.Id).Error)
		assert.Equal(t, 0, stored.OrgId, "a key created outside a team carries no organization")
	})
	t.Run("organization endpoints reject a request that names no team", func(t *testing.T) {
		for _, test := range []struct{ method, path, header, body string }{
			{"GET", "/org/context", "", ""}, {"GET", "/org/members", "", ""},
			{"GET", "/org/context", "0", ""}, {"GET", "/org/context", "-1", ""},
			{"GET", "/org/context", strconv.Itoa(team.Id + 500), ""},
		} {
			result := request(test.method, test.path, test.header, test.body)
			assert.Equal(t, 403, result.Code, test.method+test.path+" "+test.header)
		}
	})
	t.Run("teams still resolve and platform can disable and restore them", func(t *testing.T) {
		id := strconv.Itoa(team.Id)
		result := request("GET", "/org/context", id, "")
		require.Equal(t, 200, result.Code)
		assert.Contains(t, result.Body.String(), "Design team")
		for _, status := range []string{"2", "1"} {
			result = request("PUT", "/platform/organizations/"+id+"/status", "", `{"status":`+status+`,"reason":"test"}`)
			assert.Equal(t, 200, result.Code)
			assert.Contains(t, result.Body.String(), `"success":true`)
		}
	})
}
