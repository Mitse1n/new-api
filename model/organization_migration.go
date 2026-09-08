package model

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// MigratePersonalOrganizations is resumable: existing personal wallets are
// never copied again, and only rows without an organization are backfilled.
// It must run with writes quiesced before enabling organization billing.
func MigratePersonalOrganizations(db *gorm.DB) error {
	lastID := 0
	for {
		var users []User
		if err := db.Unscoped().Where("id > ?", lastID).Order("id").Limit(250).Find(&users).Error; err != nil {
			return err
		}
		if len(users) == 0 {
			return DisableInactiveOrganizationTokens(db)
		}
		for i := range users {
			user := &users[i]
			if err := db.Transaction(func(tx *gorm.DB) error {
				org, err := EnsurePersonalOrganization(tx, user)
				if err != nil {
					return err
				}
				for _, resource := range []interface{}{&Log{}, &TopUp{}, &UserSubscription{}, &SubscriptionOrder{}, &Task{}, &Midjourney{}, &QuotaData{}} {
					if err := tx.Unscoped().Model(resource).Where("user_id = ? AND (org_id IS NULL OR org_id = 0)", user.Id).Update("org_id", org.Id).Error; err != nil {
						return err
					}
				}
				// Startup runs before Redis initialization. Populate only newly assigned
				// tokens; the new cache namespace cannot contain legacy snapshots.
				return tx.Unscoped().Model(&Token{}).Where("user_id = ? AND (org_id IS NULL OR org_id = 0)", user.Id).Updates(map[string]interface{}{
					"org_id": org.Id, "org_status": org.Status, "org_group": org.Group, "org_settings": org.Settings,
				}).Error
			}); err != nil {
				return fmt.Errorf("migrating personal organization for user %d: %w", user.Id, err)
			}
			lastID = user.Id
		}
	}
}

// BackfillLogOrganizations also runs when logs live in a separate database.
// The caller handles the ClickHouse mutation path independently.
func BackfillLogOrganizations(db, logDB *gorm.DB) error {
	lastID := 0
	for {
		var orgs []Organization
		if err := db.Unscoped().Where("id > ? AND personal_user_id IS NOT NULL", lastID).Order("id").Limit(250).Find(&orgs).Error; err != nil {
			return err
		}
		if len(orgs) == 0 {
			return nil
		}
		for _, org := range orgs {
			if err := logDB.Model(&Log{}).Where("user_id = ? AND (org_id IS NULL OR org_id = 0)", *org.PersonalUserId).Update("org_id", org.Id).Error; err != nil {
				return err
			}
			lastID = org.Id
		}
	}
}

func BackfillClickHouseLogOrganizations(db, logDB *gorm.DB) error {
	lastID := 0
	for {
		var orgs []Organization
		if err := db.Unscoped().Where("id > ? AND personal_user_id IS NOT NULL", lastID).Order("id").Limit(250).Find(&orgs).Error; err != nil {
			return err
		}
		if len(orgs) == 0 {
			return nil
		}
		for _, org := range orgs {
			var count int64
			if err := logDB.Model(&Log{}).Where("user_id = ? AND org_id = 0", *org.PersonalUserId).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				if err := logDB.Exec("ALTER TABLE logs UPDATE org_id = ? WHERE user_id = ? AND org_id = 0 SETTINGS mutations_sync = 2", org.Id, *org.PersonalUserId).Error; err != nil {
					return err
				}
			}
			lastID = org.Id
		}
	}
}

// The sorting key cannot be updated in place. Startup runs before log writers
// start, so copy and rename during maintenance, retaining the original for rollback.
func migrateClickHouseOrganizationOrder(ttlDays int) error {
	var key string
	if err := LOG_DB.Raw("SELECT sorting_key FROM system.tables WHERE database = currentDatabase() AND name = 'logs'").Scan(&key).Error; err != nil {
		return err
	}
	if strings.HasPrefix(key, "org_id,") || strings.HasPrefix(key, "(org_id,") {
		return nil
	}
	create := strings.Replace(clickHouseLogCreateTableSQL(ttlDays), "logs (", "logs_tenancy_migrating (", 1)
	if err := LOG_DB.Exec(create).Error; err != nil {
		return err
	}
	// Only this migration owns the staging table; a failed copy is restartable.
	if err := LOG_DB.Exec("TRUNCATE TABLE logs_tenancy_migrating").Error; err != nil {
		return err
	}
	var columns []string
	if err := LOG_DB.Raw("SELECT name FROM system.columns WHERE database = currentDatabase() AND table = 'logs_tenancy_migrating' ORDER BY position").Scan(&columns).Error; err != nil {
		return err
	}
	for i, column := range columns {
		columns[i] = "`" + strings.ReplaceAll(column, "`", "``") + "`"
	}
	list := strings.Join(columns, ",")
	if err := LOG_DB.Exec("INSERT INTO logs_tenancy_migrating (" + list + ") SELECT " + list + " FROM logs").Error; err != nil {
		return err
	}
	var before, after int64
	if err := LOG_DB.Table("logs").Count(&before).Error; err != nil {
		return err
	}
	if err := LOG_DB.Table("logs_tenancy_migrating").Count(&after).Error; err != nil {
		return err
	}
	if before != after {
		return fmt.Errorf("ClickHouse tenancy copy count mismatch: %d != %d", before, after)
	}
	return LOG_DB.Exec("RENAME TABLE logs TO logs_tenancy_legacy, logs_tenancy_migrating TO logs").Error
}
