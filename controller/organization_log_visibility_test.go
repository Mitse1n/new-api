package controller

import (
	"net/http/httptest"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestOrganizationLogVisibility(t *testing.T) {
	var dialector gorm.Dialector = sqlite.Open(t.TempDir() + "/visibility.db")
	dialect := common.DatabaseTypeSQLite
	if dsn := os.Getenv("ORGANIZATION_API_TEST_MYSQL_DSN"); dsn != "" {
		dialector = mysql.Open(dsn)
		dialect = common.DatabaseTypeMySQL
	}
	if dsn := os.Getenv("ORGANIZATION_API_TEST_POSTGRES_DSN"); dsn != "" {
		dialector = postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
		dialect = common.DatabaseTypePostgreSQL
	}
	db, err := gorm.Open(dialector, &gorm.Config{})
	require.NoError(t, err)
	oldDB, oldLog, oldMemory := model.DB, model.LOG_DB, common.MemoryCacheEnabled
	oldDialect, oldLogDialect := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB, model.LOG_DB, common.MemoryCacheEnabled = db, db, false
	common.SetMainDatabaseType(dialect)
	common.SetLogDatabaseType(dialect)
	t.Cleanup(func() {
		model.DB, model.LOG_DB, common.MemoryCacheEnabled = oldDB, oldLog, oldMemory
		common.SetMainDatabaseType(oldDialect)
		common.SetLogDatabaseType(oldLogDialect)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})
	// External DSNs must point to disposable test databases.
	for _, resource := range []any{&model.Organization{}, &model.Log{}, &model.Channel{}} {
		require.NoError(t, db.Migrator().DropTable(resource))
		require.NoError(t, db.AutoMigrate(resource))
	}
	org := model.Organization{Id: 10, Name: "Team", Slug: "team", Kind: model.OrganizationTeam, Status: model.OrganizationActive, Group: "default"}
	require.NoError(t, db.Create(&org).Error)
	require.NoError(t, db.Create(&model.Channel{Id: 5, Name: "private-upstream-supplier"}).Error)
	other := model.NewLogOther()
	other.SetPublic("usage", "public-usage")
	other.SetAdmin("diagnostic", "admin-diagnostic")
	other.SetRoot("upstream_request_id", "root-diagnostic")
	for i, orgID := range []int{0, org.Id} {
		require.NoError(t, db.Create(&model.Log{Id: 101 + i, OrgId: orgID, UserId: 1, Type: model.LogTypeConsume, ChannelId: 5, Other: other.JSONString()}).Error)
	}
	for _, test := range []struct {
		name                 string
		role                 int
		orgID                int
		platform             bool
		channel, admin, root bool
	}{
		{"personal", common.RoleCommonUser, 0, false, false, false, false},
		{"team", common.RoleCommonUser, 10, false, true, false, false},
		{"platform admin", common.RoleAdminUser, 10, true, true, true, false},
		{"platform root", common.RoleRootUser, 10, true, true, true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(response)
			c.Set("id", 1)
			c.Set("role", test.role)
			c.Set("org_id", test.orgID)
			c.Request = httptest.NewRequest("GET", "/logs", nil)
			if test.platform {
				c.Params = gin.Params{{Key: "org_id", Value: "10"}, {Key: "resource", Value: "logs"}}
				PlatformOrganizationResources(c)
			} else {
				GetOrganizationLogs(c)
			}
			require.Equal(t, 200, response.Code)
			var payload struct {
				Success bool `json:"success"`
				Data    struct {
					Items []model.Log `json:"items"`
				} `json:"data"`
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
			require.True(t, payload.Success)
			require.Len(t, payload.Data.Items, 1)
			log := payload.Data.Items[0]
			assert.Contains(t, log.Other, "public-usage")
			if !test.platform {
				expectedChannel := ""
				if test.channel {
					expectedChannel = "private-upstream-supplier"
				}
				assert.Equal(t, expectedChannel, log.ChannelName)
			}
			if test.orgID == 0 {
				assert.Equal(t, 1, log.Id)
			}
			for value, visible := range map[string]bool{"admin-diagnostic": test.admin, "root-diagnostic": test.root} {
				if visible {
					assert.Contains(t, log.Other, value)
				} else {
					assert.NotContains(t, log.Other, value)
				}
			}
		})
	}
}
