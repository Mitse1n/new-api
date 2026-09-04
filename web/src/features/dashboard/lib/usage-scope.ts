/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import type { QuotaDataItem } from '../types'

export type UsageScope =
  | { type: 'organization' }
  | { type: 'members'; userIDs: number[] }

// Only filter rows already authorized by the server. Each usage row is counted once.
export function selectUsageRows(
  rows: QuotaDataItem[],
  scope: UsageScope
): QuotaDataItem[] {
  if (scope.type === 'organization') return rows
  const members = new Set(scope.userIDs)
  return rows.filter(
    (row) => row.user_id !== undefined && members.has(row.user_id)
  )
}
