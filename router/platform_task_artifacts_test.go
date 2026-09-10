package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPlatformTaskArtifactsRequiresAdministrator(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Task{}))
	previousDB, previousRedis := model.DB, common.RedisEnabled
	model.DB, common.RedisEnabled = db, false
	t.Cleanup(func() {
		model.DB, common.RedisEnabled = previousDB, previousRedis
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})
	adminToken, userToken := "platform-artifact-admin", "platform-artifact-user"
	require.NoError(t, db.Create(&model.User{Id: 901, Username: "artifact-admin", AffCode: "artifact-admin", Role: common.RoleAdminUser, Status: common.UserStatusEnabled, AccessToken: &adminToken}).Error)
	require.NoError(t, db.Create(&model.User{Id: 902, Username: "artifact-user", AffCode: "artifact-user", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AccessToken: &userToken}).Error)
	require.NoError(t, db.Create(&model.Task{TaskID: "other-team-task", UserId: 903, OrgId: 904, Platform: "document", Status: model.TaskStatusSuccess}).Error)
	engine := gin.New()
	SetApiRouter(engine)
	for _, test := range []struct {
		name, token string
		status      int
	}{
		{"anonymous", "", http.StatusUnauthorized},
		{"ordinary user", userToken, http.StatusForbidden},
		{"administrator", adminToken, http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/platform/tasks/other-team-task/artifacts", nil)
			if test.token != "" {
				req.Header.Set("Authorization", "Bearer "+test.token)
			}
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, req)
			assert.Equal(t, test.status, recorder.Code, recorder.Body.String())
		})
	}
}
