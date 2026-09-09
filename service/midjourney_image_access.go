package service

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// Image links are capabilities issued only by an authorized task read. Binding
// both database identity and organization prevents upstream task-ID collisions from
// exposing a different organization's image.
func BuildMidjourneyImageURL(task *model.Midjourney) (string, error) {
	if task == nil || task.OrgId < 0 || task.OrgId == 0 && task.UserId <= 0 || task.Id <= 0 || task.MjId == "" {
		return "", ErrTaskArtifactAccessInvalid
	}
	scope := fmt.Sprintf("mj:%d:%d", task.OrgId, task.Id)
	if task.OrgId == 0 {
		scope = fmt.Sprintf("mj:user:%d:%d", task.UserId, task.Id)
	}
	access, err := IssueTaskArtifactAccess(scope, "image:"+task.MjId)
	if err != nil {
		return "", err
	}
	query := url.Values{"org": {strconv.Itoa(task.OrgId)}, "task": {strconv.Itoa(task.Id)}, TaskArtifactAccessQueryParameter: {access}}
	return strings.TrimRight(system_setting.ServerAddress, "/") + "/mj/image/" + url.PathEscape(task.MjId) + "?" + query.Encode(), nil
}

func GetMidjourneyImageWithAccess(orgID, rowID int, taskID, access string) (*model.Midjourney, error) {
	if orgID < 0 || rowID <= 0 {
		return nil, ErrTaskArtifactAccessInvalid
	}
	var task model.Midjourney
	query := model.DB.Where("id = ? AND mj_id = ?", rowID, taskID)
	if orgID == 0 {
		query = query.Where("org_id IS NULL OR org_id = 0")
	} else {
		query = query.Scopes(model.OrgScope(orgID))
	}
	if err := query.First(&task).Error; err != nil {
		return nil, ErrTaskArtifactAccessInvalid
	}
	scope := fmt.Sprintf("mj:%d:%d", orgID, rowID)
	if orgID == 0 {
		if task.UserId <= 0 {
			return nil, ErrTaskArtifactAccessInvalid
		}
		scope = fmt.Sprintf("mj:user:%d:%d", task.UserId, rowID)
	}
	if !VerifyTaskArtifactAccess(access, scope, "image:"+taskID) {
		return nil, ErrTaskArtifactAccessInvalid
	}
	if orgID > 0 {
		var org model.Organization
		if err := model.DB.Where("id = ? AND status = ?", orgID, model.OrganizationActive).First(&org).Error; err != nil {
			return nil, ErrTaskArtifactAccessInvalid
		}
	}
	return &task, nil
}
