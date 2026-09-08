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
	tasks := []model.Midjourney{{Id: 11, OrgId: 1, UserId: 7, MjId: "upstream-same-id", ImageUrl: "https://a.example/image"},
		{Id: 22, OrgId: 2, UserId: 8, MjId: "upstream-same-id", ImageUrl: "https://b.example/image"},
		{Id: 33, UserId: 7, MjId: "upstream-same-id", ImageUrl: "https://own.example/image"}}
	require.NoError(t, db.Create(&tasks).Error)
	link, err := BuildMidjourneyImageURL(&tasks[0])
	require.NoError(t, err)
	parsed, err := url.Parse(link)
	require.NoError(t, err)
	orgID, err := strconv.Atoi(parsed.Query().Get("org"))
	require.NoError(t, err)
	userID, err := strconv.Atoi(parsed.Query().Get("user"))
	require.NoError(t, err)
	rowID, err := strconv.Atoi(parsed.Query().Get("task"))
	require.NoError(t, err)
	access := parsed.Query().Get(TaskArtifactAccessQueryParameter)
	task, err := GetMidjourneyImageWithAccess(orgID, userID, rowID, tasks[0].MjId, access)
	require.NoError(t, err)
	assert.Equal(t, tasks[0].ImageUrl, task.ImageUrl)

	// A task owned by an account rather than a team gets its own capability,
	// bound to that user, and the team capability must not open it.
	ownLink, err := BuildMidjourneyImageURL(&tasks[2])
	require.NoError(t, err)
	ownParsed, err := url.Parse(ownLink)
	require.NoError(t, err)
	ownAccess := ownParsed.Query().Get(TaskArtifactAccessQueryParameter)
	assert.Equal(t, "0", ownParsed.Query().Get("org"))
	ownTask, err := GetMidjourneyImageWithAccess(0, 7, 33, tasks[2].MjId, ownAccess)
	require.NoError(t, err)
	assert.Equal(t, tasks[2].ImageUrl, ownTask.ImageUrl)

	for _, test := range []struct {
		name           string
		org, user, row int
		id, access     string
	}{
		{"foreign organization row", 2, 8, 22, tasks[0].MjId, access},
		{"foreign upstream id", 1, 7, 11, "another-id", access},
		{"missing capability", 1, 7, 11, tasks[0].MjId, ""},
		{"organization dropped from capability", 0, 7, 11, tasks[0].MjId, access},
		{"account capability replayed as team", 1, 7, 33, tasks[2].MjId, ownAccess},
		{"account row read by another user", 0, 8, 33, tasks[2].MjId, ownAccess},
		{"account row with no user", 0, 0, 33, tasks[2].MjId, ownAccess},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := GetMidjourneyImageWithAccess(test.org, test.user, test.row, test.id, test.access)
			assert.ErrorIs(t, err, ErrTaskArtifactAccessInvalid)
		})
	}
	require.NoError(t, db.Model(&model.Organization{}).Where("id = ?", 1).Update("status", model.OrganizationDisabled).Error)
	_, err = GetMidjourneyImageWithAccess(1, 7, 11, tasks[0].MjId, access)
	assert.ErrorIs(t, err, ErrTaskArtifactAccessInvalid)
}
