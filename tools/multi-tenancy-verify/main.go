// Command multi-tenancy-verify validates startup migration on disposable fixtures.
package main

import (
	"fmt"
	"os"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/authz"
)

func main() {
	if os.Getenv("TENANCY_VERIFY_STARTUP") != "1" {
		panic("set TENANCY_VERIFY_STARTUP=1 and point SQL_DSN/SQLITE_PATH to a disposable verification database")
	}
	common.IsMasterNode = true
	common.RedisEnabled = false
	if path := os.Getenv("SQLITE_PATH"); path != "" {
		common.SQLitePath = path
	}
	for run := 1; run <= 2; run++ {
		if err := model.InitDB(); err != nil {
			panic(err)
		}
		if err := model.InitLogDB(); err != nil {
			panic(err)
		}
		if err := authz.Init(model.DB); err != nil {
			panic(err)
		}
		var users []model.User
		if err := model.DB.Unscoped().Find(&users).Error; err != nil {
			panic(err)
		}
		for _, user := range users {
			org, err := model.GetPersonalOrganization(user.Id)
			if err != nil {
				panic(err)
			}
			if org.Quota != int64(user.Quota) || org.UsedQuota != int64(user.UsedQuota) || org.Group != user.Group {
				panic("personal wallet or group changed during migration")
			}
			var count int64
			if err := model.DB.Model(&model.OrganizationMember{}).Where("org_id = ? AND user_id = ? AND role = ?", org.Id, user.Id, model.OrgRoleOwner).Count(&count).Error; err != nil || count != 1 {
				panic("personal owner membership was not preserved")
			}
			for _, resource := range []interface{}{&model.Token{}, &model.TopUp{}, &model.UserSubscription{}, &model.SubscriptionOrder{}, &model.Task{}, &model.Midjourney{}, &model.QuotaData{}} {
				if err := model.DB.Unscoped().Model(resource).Where("user_id = ? AND (org_id IS NULL OR org_id <> ?)", user.Id, org.Id).Count(&count).Error; err != nil || count != 0 {
					panic(fmt.Sprintf("resource ownership failed: %T, %v", resource, err))
				}
			}
			if err := model.LOG_DB.Model(&model.Log{}).Where("user_id = ? AND (org_id IS NULL OR org_id <> ?)", user.Id, org.Id).Count(&count).Error; err != nil || count != 0 {
				panic("log ownership failed")
			}
			if user.Username == "legacy-owner" {
				if user.Quota != 123456789012 || user.UsedQuota != 321 {
					panic("released wallet values were changed")
				}
				var token model.Token
				if err := model.DB.Where("name = ?", "legacy").First(&token).Error; err != nil || token.Key != "legacy-migration-key" || token.RemainQuota != 12345 {
					panic("released token values were changed")
				}
				var logs []model.Log
				if err := model.LOG_DB.Where("request_id = ?", "legacy-log").Find(&logs).Error; err != nil || len(logs) != 1 || logs[0].Quota != 321 {
					panic("released log values were changed")
				}
			}
		}
		for _, spec := range []struct {
			resource interface{}
			index    string
		}{{&model.Organization{}, "idx_organizations_slug"}, {&model.Organization{}, "idx_organizations_personal_user_id"}, {&model.OrganizationMember{}, "idx_org_member"}, {&model.OrganizationCharge{}, "idx_organization_charges_request_id"}} {
			if !model.DB.Migrator().HasIndex(spec.resource, spec.index) {
				panic("missing index: " + spec.index)
			}
		}
		fmt.Printf("STARTUP_PASS run=%d users=%d main=%s log=%s\n", run, len(users), common.MainDatabaseType(), common.LogDatabaseType())
		logSQL, _ := model.LOG_DB.DB()
		mainSQL, _ := model.DB.DB()
		if logSQL != mainSQL {
			_ = logSQL.Close()
		}
		_ = mainSQL.Close()
	}
}
