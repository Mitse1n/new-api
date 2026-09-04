package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
)

func organizationUsageScope(c *gin.Context) model.OrganizationResourceScope {
	scope := model.OrganizationResourceScope{OrgID: c.GetInt("org_id"), UserID: c.GetInt("id"), AllMembers: authz.CanOrg(c.GetInt("id"), c.GetInt("org_id"), c.GetString("org_role"), authz.Permission{Resource: "org.usage", Action: "read_all"})}
	if scope.AllMembers {
		if userID, err := strconv.Atoi(c.Query("user_id")); err == nil && userID > 0 {
			scope.UserID, scope.AllMembers = userID, false
		}
	}
	return scope
}

func GetOrganizationLogs(c *gin.Context) {
	page := common.GetPageQuery(c)
	logType, _ := strconv.Atoi(c.Query("type"))
	start, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	end, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	channel, _ := strconv.Atoi(c.Query("channel"))
	logs, total, err := model.GetAllLogs(logType, start, end, c.Query("model_name"), c.Query("username"), c.Query("token_name"), page.GetStartIdx(), page.GetPageSize(), channel, c.Query("group"), c.Query("request_id"), c.Query("upstream_request_id"), organizationUsageScope(c).Apply)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.FormatOrganizationLogs(logs)
	page.SetTotal(int(total))
	page.SetItems(logs)
	common.ApiSuccess(c, page)
}

func GetOrganizationLogStats(c *gin.Context) {
	start, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	end, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	channel, _ := strconv.Atoi(c.Query("channel"))
	stat, err := model.SumUsedQuota(model.LogTypeConsume, start, end, c.Query("model_name"), c.Query("username"), c.Query("token_name"), channel, c.Query("group"), organizationUsageScope(c).Apply)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, stat)
}
