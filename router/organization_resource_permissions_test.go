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

func TestOrganizationResourceReadsRequireReadPermission(t *testing.T) {
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
	user := model.User{Id: 921, Username: "read-owner", AffCode: "read-owner", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, AccessToken: &token}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&model.Organization{Id: 931, OwnerId: user.Id, Name: "Read team", Slug: "read-team", Kind: model.OrganizationTeam, Status: model.OrganizationActive, Group: "default", Settings: "{}"}).Error)
	require.NoError(t, db.Create(&model.OrganizationMember{OrgId: 931, UserId: user.Id, Role: model.OrgRoleOwner, Status: model.OrganizationActive}).Error)
	require.NoError(t, authz.Init(db))
	engine := gin.New()
	SetApiRouter(engine)
	for _, test := range []struct{ resource, path string }{
		{"org.token", "/api/token/"},
		{"org.billing", "/api/user/topup/self"},
		{"org.usage", "/api/data/self?start_timestamp=1&end_timestamp=2"},
		{"org.usage", "/api/data/flow/self?start_timestamp=1&end_timestamp=2"},
		{"org.usage", "/api/log/self"},
	} {
		t.Run(test.path, func(t *testing.T) {
			deny := model.CasbinRule{Ptype: "p", V0: authz.UserSubject(user.Id), V1: "org:931", V2: test.resource, V3: "write", V4: authz.EffectDeny}
			require.NoError(t, db.Create(&deny).Error)
			require.NoError(t, authz.ReloadPolicy())
			request := func(method, path string) *httptest.ResponseRecorder {
				req := httptest.NewRequest(method, path, nil)
				req.Header.Set("Authorization", "Bearer "+token)
				req.Header.Set("X-Org-Id", "931")
				recorder := httptest.NewRecorder()
				engine.ServeHTTP(recorder, req)
				return recorder
			}
			result := request(http.MethodGet, test.path)
			assert.Equal(t, http.StatusOK, result.Code, result.Body.String())
			assert.Contains(t, result.Body.String(), `"success":true`)
			if test.resource == "org.token" {
				result = request(http.MethodPost, "/api/token/")
				assert.Equal(t, http.StatusForbidden, result.Code)
			}
			require.NoError(t, db.Model(&deny).Update("v3", "read").Error)
			require.NoError(t, authz.ReloadPolicy())
			result = request(http.MethodGet, test.path)
			assert.Equal(t, http.StatusForbidden, result.Code)
			assert.Contains(t, result.Body.String(), "ORG_FORBIDDEN")
			require.NoError(t, db.Delete(&deny).Error)
			require.NoError(t, authz.ReloadPolicy())
		})
	}
}
