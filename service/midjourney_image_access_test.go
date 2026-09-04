package service

import (
	"net/url"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMidjourneyImageCapabilityRejectsForeignOrganizationAndDisabledOrganization(t *testing.T) {
	previousDB, previousSecret := model.DB, common.CryptoSecret
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/mj-access.db"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Organization{}, &model.Midjourney{}))
	model.DB, common.CryptoSecret = db, "mj-access-fixture"
	t.Cleanup(func() {
		model.DB, common.CryptoSecret = previousDB, previousSecret
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.Create(&[]model.Organization{{Id: 1, Name: "A", Slug: "a", Kind: "team", Status: 1}, {Id: 2, Name: "B", Slug: "b", Kind: "team", Status: 1}}).Error)
	tasks := []model.Midjourney{{Id: 11, OrgId: 1, MjId: "upstream-same-id", ImageUrl: "https://a.example/image"}, {Id: 22, OrgId: 2, MjId: "upstream-same-id", ImageUrl: "https://b.example/image"}}
	require.NoError(t, db.Create(&tasks).Error)
	link, err := BuildMidjourneyImageURL(&tasks[0])
	require.NoError(t, err)
	parsed, err := url.Parse(link)
	require.NoError(t, err)
	orgID, err := strconv.Atoi(parsed.Query().Get("org"))
	require.NoError(t, err)
	rowID, err := strconv.Atoi(parsed.Query().Get("task"))
	require.NoError(t, err)
	access := parsed.Query().Get(TaskArtifactAccessQueryParameter)
	task, err := GetMidjourneyImageWithAccess(orgID, rowID, tasks[0].MjId, access)
	require.NoError(t, err)
	assert.Equal(t, tasks[0].ImageUrl, task.ImageUrl)
	for _, test := range []struct {
		org, row   int
		id, access string
	}{{2, 22, tasks[0].MjId, access}, {1, 11, "another-id", access}, {1, 11, tasks[0].MjId, ""}, {0, 11, tasks[0].MjId, access}} {
		_, err = GetMidjourneyImageWithAccess(test.org, test.row, test.id, test.access)
		assert.ErrorIs(t, err, ErrTaskArtifactAccessInvalid)
	}
	require.NoError(t, db.Model(&model.Organization{}).Where("id = ?", 1).Update("status", model.OrganizationDisabled).Error)
	_, err = GetMidjourneyImageWithAccess(1, 11, tasks[0].MjId, access)
	assert.ErrorIs(t, err, ErrTaskArtifactAccessInvalid)
}
