package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestOrganizationResourceRoutesEnforceMemberRole(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Organization{}, &model.OrganizationMember{}, &model.OrganizationAudit{}, &model.Token{}, &model.TopUp{}, &model.QuotaData{}, &model.Log{}, &model.CasbinRule{}, &model.AuthzRole{}))
	previousDB, previousLogDB, previousRedis, previousMaster := model.DB, model.LOG_DB, common.RedisEnabled, common.IsMasterNode
	model.DB, model.LOG_DB, common.RedisEnabled, common.IsMasterNode = db, db, false, true
	t.Cleanup(func() {
		model.DB, model.LOG_DB, common.RedisEnabled, common.IsMasterNode = previousDB, previousLogDB, previousRedis, previousMaster
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})
	token := "organization-read-permission"
	user := model.User{Id: 921, Username: "read-member", AffCode: "read-member", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, AccessToken: &token}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&model.Organization{Id: 931, OwnerId: 920, Name: "Read team", Slug: "read-team", Status: model.OrganizationActive, Group: "default", Settings: "{}"}).Error)
	require.NoError(t, db.Create(&model.OrganizationMember{OrgId: 931, UserId: user.Id, Role: model.OrgRoleMember, Status: model.OrganizationActive}).Error)
	require.NoError(t, authz.Init(db))
	engine := gin.New()
	SetApiRouter(engine)
	for _, test := range []struct {
		path   string
		status int
	}{
		{"/api/token/", http.StatusOK},
		{"/api/user/topup/self", http.StatusForbidden},
		{"/api/data/self?start_timestamp=1&end_timestamp=2", http.StatusOK},
		{"/api/data/flow/self?start_timestamp=1&end_timestamp=2", http.StatusOK},
		{"/api/log/self", http.StatusOK},
		{"/api/org/settings", http.StatusForbidden},
	} {
		t.Run(test.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, test.path, nil)
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("X-Org-Id", "931")
			result := httptest.NewRecorder()
			engine.ServeHTTP(result, req)
			assert.Equal(t, test.status, result.Code, result.Body.String())
			if test.status == http.StatusOK {
				assert.Contains(t, result.Body.String(), `"success":true`)
			} else {
				assert.Contains(t, result.Body.String(), "ORG_FORBIDDEN")
			}
		})
	}
}
