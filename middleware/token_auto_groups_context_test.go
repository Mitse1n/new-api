package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTokenAutoGroupsContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	return ctx
}

func TestSetupContextForTokenPreservesCustomAutoGroupsOrder(t *testing.T) {
	ctx := newTokenAutoGroupsContext()
	token := &model.Token{Id: 1, UserId: 2, AutoGroups: `["vip","default"]`}

	require.NoError(t, SetupContextForToken(ctx, token))
	value, ok := common.GetContextKey(ctx, constant.ContextKeyTokenAutoGroups)
	require.True(t, ok)
	assert.Equal(t, []string{"vip", "default"}, value)
}

func TestSetupContextForTokenTreatsStoredEmptyArrayAsInheritance(t *testing.T) {
	ctx := newTokenAutoGroupsContext()
	token := &model.Token{Id: 1, UserId: 2, AutoGroups: `[]`}

	require.NoError(t, SetupContextForToken(ctx, token))
	_, ok := common.GetContextKey(ctx, constant.ContextKeyTokenAutoGroups)
	assert.False(t, ok)
}

func TestSetupContextForTokenMalformedAutoGroupsFailsClosed(t *testing.T) {
	ctx := newTokenAutoGroupsContext()
	token := &model.Token{Id: 1, UserId: 2, AutoGroups: `not-json`}

	require.NoError(t, SetupContextForToken(ctx, token))
	value, ok := common.GetContextKey(ctx, constant.ContextKeyTokenAutoGroups)
	require.True(t, ok)
	assert.Equal(t, []string{}, value)
}

func TestSetupContextForTokenIntersectsOrganizationModelLimits(t *testing.T) {
	ctx := newTokenAutoGroupsContext()
	setupDashboardAuthMiddlewareTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Organization{}, &model.OrganizationMember{}))
	org := model.Organization{Id: 42, Name: "Team", Slug: "team", Kind: model.OrganizationTeam, Status: model.OrganizationActive, Settings: `{"allowed_models":["allowed","outside-key"]}`}
	require.NoError(t, model.DB.Create(&org).Error)
	require.NoError(t, model.DB.Create(&model.OrganizationMember{OrgId: 42, UserId: 3, Status: model.OrganizationActive}).Error)
	token := &model.Token{Id: 9, UserId: 3, OrgId: 42, ModelLimitsEnabled: true, ModelLimits: "allowed,other"}
	require.NoError(t, SetupContextForToken(ctx, token))
	assert.Equal(t, 42, common.GetContextKeyInt(ctx, constant.ContextKeyOrgId))
	assert.True(t, ctx.GetBool("token_model_limit_enabled"))
	assert.Equal(t, map[string]bool{"allowed": true}, ctx.MustGet("token_model_limit"))
}
