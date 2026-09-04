// Command multi-tenancy-migrate runs the same migration as startup without
// accepting traffic or starting background billing/cleanup workers.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/authz"
)

func main() {
	action := flag.String("action", "verify", "migrate or verify (verify does not migrate)")
	offline := flag.Bool("offline", false, "confirm all application writers and workers are stopped")
	snapshot := flag.String("snapshot", "", "verified snapshot directory required for migration")
	flag.Parse()
	if err := run(*action, *offline, *snapshot); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(action string, offline bool, snapshot string) error {
	if action != "migrate" && action != "verify" {
		return errors.New("action must be migrate or verify")
	}
	if !offline {
		return errors.New("stop all writers and workers, then pass -offline")
	}
	if action == "migrate" {
		if snapshot == "" {
			return errors.New("create an offline snapshot first and pass -snapshot")
		}
		file, err := os.Open(filepath.Join(snapshot, "manifest.json"))
		if err != nil {
			return err
		}
		var manifest struct {
			Format int               `json:"format"`
			Files  map[string]string `json:"files"`
		}
		err = common.DecodeJson(file, &manifest)
		_ = file.Close()
		if err != nil {
			return err
		}
		if manifest.Format != 1 || len(manifest.Files) == 0 {
			return errors.New("snapshot manifest is empty or unsupported")
		}
		for name, expected := range manifest.Files {
			if filepath.Base(name) != name {
				return errors.New("invalid snapshot filename")
			}
			source, err := os.Open(filepath.Join(snapshot, name))
			if err != nil {
				return err
			}
			sum := sha256.New()
			_, err = io.Copy(sum, source)
			_ = source.Close()
			if err != nil {
				return err
			}
			if hex.EncodeToString(sum.Sum(nil)) != expected {
				return fmt.Errorf("snapshot checksum mismatch: %s", name)
			}
		}
	}
	common.IsMasterNode = action == "migrate"
	common.RedisEnabled = false
	if path := os.Getenv("SQLITE_PATH"); path != "" {
		common.SQLitePath = path
	}
	if err := model.InitDB(); err != nil {
		return err
	}
	defer model.CloseDB()
	if err := model.InitLogDB(); err != nil {
		return err
	}
	if action == "migrate" {
		if err := authz.Init(model.DB); err != nil {
			return err
		}
	}
	var users []model.User
	if err := model.DB.Unscoped().Find(&users).Error; err != nil {
		return err
	}
	for _, user := range users {
		org, err := model.GetPersonalOrganization(user.Id)
		if err != nil {
			return err
		}
		if org.Id != user.PersonalOrgId || org.Quota != int64(user.Quota) {
			return fmt.Errorf("personal ownership/wallet mismatch for user %d", user.Id)
		}
		var owners int64
		if err := model.DB.Model(&model.OrganizationMember{}).Where("org_id = ? AND user_id = ? AND role = ?", org.Id, user.Id, model.OrgRoleOwner).Count(&owners).Error; err != nil {
			return err
		}
		if owners != 1 {
			return fmt.Errorf("personal owner membership mismatch for user %d", user.Id)
		}
	}
	for _, resource := range []interface{}{&model.Token{}, &model.TopUp{}, &model.UserSubscription{}, &model.SubscriptionOrder{}, &model.Task{}, &model.Midjourney{}, &model.QuotaData{}} {
		var orphans int64
		if err := model.DB.Unscoped().Model(resource).Where("user_id > 0 AND (org_id IS NULL OR org_id <= 0)").Count(&orphans).Error; err != nil {
			return err
		}
		if orphans != 0 {
			return fmt.Errorf("%T has %d unassigned resources", resource, orphans)
		}
	}
	var logOrphans int64
	if err := model.LOG_DB.Model(&model.Log{}).Where("user_id > 0 AND (org_id IS NULL OR org_id <= 0)").Count(&logOrphans).Error; err != nil {
		return err
	}
	if logOrphans != 0 {
		return fmt.Errorf("log database has %d unassigned resources", logOrphans)
	}
	fmt.Printf("TENANCY_VERIFIED action=%s users=%d main=%s log=%s\n", action, len(users), common.MainDatabaseType(), common.LogDatabaseType())
	return nil
}
