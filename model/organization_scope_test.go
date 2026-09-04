package model

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrganizationScopeRejectsMissingOrganizationAcrossResourceTypes(t *testing.T) {
	db := organizationTestDatabase(t)
	for _, resource := range OrganizationResources() {
		t.Run(reflect.TypeOf(resource).Elem().Name(), func(t *testing.T) {
			require.NoError(t, db.Migrator().DropTable(resource))
			require.NoError(t, db.AutoMigrate(resource))
			for _, orgID := range []int{10, 20} {
				item := reflect.New(reflect.TypeOf(resource).Elem())
				fields := item.Elem()
				field := fields.FieldByName("OrgId")
				if field.Kind() == reflect.Pointer {
					value := orgID
					field.Set(reflect.ValueOf(&value))
				} else {
					field.SetInt(int64(orgID))
				}
				// Valid, deterministic unique identities let the same real query contract
				// run against every persistence model on each supported SQL dialect.
				for _, name := range []string{"Key", "RequestId", "TradeNo", "EventKey", "TokenHash"} {
					if f := fields.FieldByName(name); f.IsValid() && f.CanSet() && f.Kind() == reflect.String {
						f.SetString(fmt.Sprintf("scope-%s-%d", fields.Type().Name(), orgID))
					}
				}
				require.NoError(t, db.Create(item.Interface()).Error)
			}
			for _, scope := range []struct {
				orgID int
				count int64
			}{{10, 1}, {20, 1}, {30, 0}, {0, 0}, {-1, 0}} {
				var count int64
				require.NoError(t, db.Model(resource).Scopes(OrgScope(scope.orgID)).Count(&count).Error)
				assert.Equal(t, scope.count, count, "org_id=%d", scope.orgID)
			}
		})
	}
}
