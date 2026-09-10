package relay

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestMidjourneyImagesKeepPublicLinks(t *testing.T) {
	var dialector gorm.Dialector = sqlite.Open(t.TempDir() + "/images.db")
	// External DSNs must point to an empty, disposable database.
	if dsn := os.Getenv("MJ_IMAGE_TEST_MYSQL_DSN"); dsn != "" {
		dialector = mysql.Open(dsn)
	}
	if dsn := os.Getenv("MJ_IMAGE_TEST_POSTGRES_DSN"); dsn != "" {
		dialector = postgres.Open(dsn)
	}
	db, err := gorm.Open(dialector, &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Midjourney{}, &model.Channel{}))
	oldDB, oldCache := model.DB, common.MemoryCacheEnabled
	oldForward, oldAddress := setting.MjForwardUrlEnabled, system_setting.ServerAddress
	fetch := system_setting.GetFetchSetting()
	oldProtection := fetch.EnableSSRFProtection
	model.DB, common.MemoryCacheEnabled = db, false
	setting.MjForwardUrlEnabled, system_setting.ServerAddress = true, "https://gateway.example"
	fetch.EnableSSRFProtection = false // Allow the local upstream fixture.
	if service.GetHttpClient() == nil {
		service.InitHttpClient()
	}
	t.Cleanup(func() {
		model.DB, common.MemoryCacheEnabled = oldDB, oldCache
		setting.MjForwardUrlEnabled, system_setting.ServerAddress = oldForward, oldAddress
		fetch.EnableSSRFProtection = oldProtection
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("fixture-image"))
	}))
	defer upstream.Close()
	router := gin.New()
	router.GET("/mj/image/:id", RelayMidjourneyImage)
	for _, test := range []struct {
		name  string
		orgID int
	}{{"personal", 0}, {"organization", 10}} {
		t.Run(test.name, func(t *testing.T) {
			task := model.Midjourney{UserId: 7, OrgId: test.orgID, MjId: test.name, ImageUrl: upstream.URL, Status: "SUCCESS"}
			require.NoError(t, db.Create(&task).Error)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			dto := coverMidjourneyTaskDto(c, &task)
			assert.Equal(t, "https://gateway.example/mj/image/"+test.name, dto.ImageUrl)
			task.Status = "IN_PROGRESS"
			dto = coverMidjourneyTaskDto(c, &task)
			assert.Contains(t, dto.ImageUrl, "/mj/image/"+test.name+"?rand=")
			response := httptest.NewRecorder()
			// No session, organization header, task row ID, or signature.
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/mj/image/"+test.name, nil))
			assert.Equal(t, http.StatusOK, response.Code)
			assert.Equal(t, "image/png", response.Header().Get("Content-Type"))
			assert.Equal(t, "fixture-image", response.Body.String())
		})
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/mj/image/missing", nil))
	assert.Equal(t, http.StatusBadRequest, response.Code)
	assert.JSONEq(t, `{"error":"midjourney_task_not_found"}`, response.Body.String())
}
