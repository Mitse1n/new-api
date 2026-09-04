package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	"gorm.io/gorm"
)

func TestOrganizationAPIsEnforceScopeAndFreshMembership(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/isolation.db"), &gorm.Config{})
	require.NoError(t, err)
	oldDB, oldLog, oldRedis, oldMaster := model.DB, model.LOG_DB, common.RedisEnabled, common.IsMasterNode
	model.DB, model.LOG_DB, common.RedisEnabled, common.IsMasterNode = db, db, false, true
	t.Cleanup(func() {
		model.DB, model.LOG_DB, common.RedisEnabled, common.IsMasterNode = oldDB, oldLog, oldRedis, oldMaster
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Organization{}, &model.OrganizationMember{}, &model.OrganizationAudit{}, &model.Token{}, &model.Log{}, &model.Task{}, &model.CasbinRule{}, &model.AuthzRole{}))
	require.NoError(t, authz.Init(db))
	users := []model.User{{Id: 1, Username: "alpha-owner", AffCode: "ao"}, {Id: 2, Username: "alpha-member", AffCode: "am"}, {Id: 3, Username: "beta-owner", AffCode: "bo"}}
	require.NoError(t, db.Create(&users).Error)
	require.NoError(t, db.Create(&[]model.Organization{{Id: 10, Name: "Alpha", Slug: "alpha", Kind: "team", Status: 1, OwnerId: 1, Group: "default"}, {Id: 20, Name: "Beta", Slug: "beta", Kind: "team", Status: 1, OwnerId: 3, Group: "default"}}).Error)
	require.NoError(t, db.Create(&[]model.OrganizationMember{{OrgId: 10, UserId: 1, Role: "owner", Status: 1}, {OrgId: 10, UserId: 2, Role: "member", Status: 1}, {OrgId: 20, UserId: 3, Role: "owner", Status: 1}}).Error)
	keys := []model.Token{{OrgId: 10, UserId: 1, Name: "alpha-owned", Key: "secret-alpha-owned"}, {OrgId: 10, UserId: 2, Name: "alpha-member", Key: "secret-alpha-member"}, {OrgId: 20, UserId: 3, Name: "beta-private", Key: "secret-beta-private"}}
	require.NoError(t, db.Create(&keys).Error)
	require.NoError(t, db.Create(&[]model.Log{{OrgId: 10, UserId: 1, Username: "alpha-owner", Type: 2, Quota: 10, RequestId: "alpha-owner-log"}, {OrgId: 10, UserId: 2, Username: "alpha-member", Type: 2, Quota: 20, RequestId: "alpha-member-log"}, {OrgId: 20, UserId: 3, Username: "beta-owner", Type: 2, Quota: 30, RequestId: "beta-private-log"}}).Error)
	r := gin.New()
	// The fixture supplies a verified global identity, then exercises the same
	// organization context/permission middleware used behind session authentication.
	r.Use(func(c *gin.Context) {
		id, _ := strconv.Atoi(c.GetHeader("Test-User"))
		c.Set("id", id)
		c.Set("role", common.RoleRootUser)
		c.Next()
	}, middleware.OrganizationContext())
	r.GET("/tokens", middleware.RequireOrgPermission("org.token", "read"), GetAllTokens)
	r.GET("/tokens/:id", middleware.RequireOrgPermission("org.token", "read"), GetToken)
	r.POST("/tokens/batch", middleware.RequireOrgPermission("org.token", "write"), DeleteTokenBatch)
	r.GET("/logs", middleware.RequireOrgPermission("org.usage", "read"), GetOrganizationLogs)
	r.GET("/task/:key", middleware.RequireOrgPermission("org.usage", "read"), GetTask)
	for _, test := range []struct {
		user            int
		org, path       string
		status          int
		visible, hidden string
	}{
		{1, "10", "/tokens", 200, "alpha-member", "beta-private"},
		{2, "10", "/tokens?user_id=1", 200, "alpha-member", "alpha-owned"},
		{1, "20", "/tokens", 403, "ORG_UNAVAILABLE", "beta-private"},
		{1, "999", "/tokens", 403, "ORG_UNAVAILABLE", "Beta"},
		{1, "0", "/tokens", 403, "ORG_UNAVAILABLE", "beta-private"},
		{1, "10", "/tokens?keyword=beta&page_size=1&p=2", 200, "items", "beta-private"},
		{2, "10", "/logs?user_id=3&page_size=100", 200, "alpha-member-log", "alpha-owner-log"},
		{1, "10", "/logs?username=beta-owner", 200, "items", "beta-private-log"},
		{1, "10", "/tokens/" + strconv.Itoa(keys[2].Id), 200, "false", "secret-beta-private"},
	} {
		t.Run(fmt.Sprintf("%d-%s-%s", test.user, test.org, test.path), func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			request.Header.Set("Test-User", strconv.Itoa(test.user))
			request.Header.Set("X-Org-Id", test.org)
			response := httptest.NewRecorder()
			r.ServeHTTP(response, request)
			assert.Equal(t, test.status, response.Code)
			assert.Contains(t, response.Body.String(), test.visible)
			assert.NotContains(t, response.Body.String(), test.hidden)
			for _, key := range keys {
				assert.NotContains(t, response.Body.String(), key.Key)
			}
		})
	}
	batch, _ := common.Marshal(map[string]interface{}{"ids": []int{keys[0].Id, keys[2].Id}})
	request := httptest.NewRequest(http.MethodPost, "/tokens/batch", bytes.NewReader(batch))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Test-User", "1")
	request.Header.Set("X-Org-Id", "10")
	response := httptest.NewRecorder()
	r.ServeHTTP(response, request)
	assert.Contains(t, response.Body.String(), `"success":false`)
	var count int64
	require.NoError(t, db.Model(&model.Token{}).Count(&count).Error)
	assert.Equal(t, int64(3), count, "cross-organization batch must be all-or-nothing")
	require.NoError(t, db.Model(&model.OrganizationMember{}).Where("org_id = ? AND user_id = ?", 10, 2).Update("status", 2).Error)
	request = httptest.NewRequest(http.MethodGet, "/tokens", nil)
	request.Header.Set("Test-User", "2")
	request.Header.Set("X-Org-Id", "10")
	response = httptest.NewRecorder()
	r.ServeHTTP(response, request)
	assert.Equal(t, 403, response.Code)
}
