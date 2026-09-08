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
// the database identity together with the owning scope prevents upstream
// task-ID collisions from exposing another owner's image. A task with no
// organization is owned by its user, so the user is the scope in that case.
func BuildMidjourneyImageURL(task *model.Midjourney) (string, error) {
	if task == nil || task.Id <= 0 || task.MjId == "" {
		return "", ErrTaskArtifactAccessInvalid
	}
	scope, err := midjourneyImageScope(task.OrgId, task.UserId, task.Id)
	if err != nil {
		return "", err
	}
	access, err := IssueTaskArtifactAccess(scope, "image:"+task.MjId)
	if err != nil {
		return "", err
	}
	query := url.Values{"org": {strconv.Itoa(task.OrgId)}, "user": {strconv.Itoa(task.UserId)},
		"task": {strconv.Itoa(task.Id)}, TaskArtifactAccessQueryParameter: {access}}
	return strings.TrimRight(system_setting.ServerAddress, "/") + "/mj/image/" + url.PathEscape(task.MjId) + "?" + query.Encode(), nil
}

func GetMidjourneyImageWithAccess(orgID, userID, rowID int, taskID, access string) (*model.Midjourney, error) {
	scope, err := midjourneyImageScope(orgID, userID, rowID)
	if err != nil {
		return nil, err
	}
	if rowID <= 0 || !VerifyTaskArtifactAccess(access, scope, "image:"+taskID) {
		return nil, ErrTaskArtifactAccessInvalid
	}
	if orgID > 0 {
		var org model.Organization
		if err := model.DB.Where("id = ? AND status = ?", orgID, model.OrganizationActive).First(&org).Error; err != nil {
			return nil, ErrTaskArtifactAccessInvalid
		}
	}
	var task model.Midjourney
	query := model.OrganizationResourceScope{OrgID: orgID, UserID: userID, AllMembers: orgID > 0}.Apply(model.DB)
	if err := query.Where("id = ? AND mj_id = ?", rowID, taskID).First(&task).Error; err != nil {
		return nil, ErrTaskArtifactAccessInvalid
	}
	return &task, nil
}

// midjourneyImageScope names the owner an image capability is bound to: the
// organization for a team task, otherwise the user who created it.
func midjourneyImageScope(orgID, userID, rowID int) (string, error) {
	if rowID <= 0 {
		return "", ErrTaskArtifactAccessInvalid
	}
	if orgID > 0 {
		return fmt.Sprintf("mj:org:%d:%d", orgID, rowID), nil
	}
	if userID <= 0 {
		return "", ErrTaskArtifactAccessInvalid
	}
	return fmt.Sprintf("mj:user:%d:%d", userID, rowID), nil
}
