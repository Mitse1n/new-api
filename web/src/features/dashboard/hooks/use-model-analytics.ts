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
import { useQuery } from '@tanstack/react-query'
import { useMemo } from 'react'

import { getUserQuotaDates } from '@/features/dashboard/api'
import {
  buildQueryParams,
  getDefaultDays,
} from '@/features/dashboard/lib/filters'
import {
  selectUsageRows,
  type UsageScope,
} from '@/features/dashboard/lib/usage-scope'
import type { DashboardFilters } from '@/features/dashboard/types'
import { usePlatformView } from '@/features/organizations/platform-view'
import { ROLE } from '@/lib/roles'
import { computeTimeRange } from '@/lib/time'
import { useAuthStore } from '@/stores/auth-store'
import { useOrganizationStore } from '@/stores/organization-store'

export function useModelAnalytics(
  filters: DashboardFilters,
  selectedScope: UsageScope
) {
  const user = useAuthStore((state) => state.auth.user)
  const context = useOrganizationStore((state) => state.context)
  const epoch = useOrganizationStore((state) => state.epoch)
  const platform = usePlatformView()
  const isPlatformAdmin = platform && (user?.role ?? 0) >= ROLE.ADMIN
  const canCompare =
    !platform &&
    context?.organization?.kind === 'team' &&
    context.capabilities.org['org.usage']?.read_all === true
  const timeRange = useMemo(
    () =>
      computeTimeRange(
        getDefaultDays(filters.time_granularity),
        filters.start_timestamp,
        filters.end_timestamp
      ),
    [filters]
  )
  const params = buildQueryParams(timeRange, filters)
  const query = useQuery({
    queryKey: [
      'model-analytics',
      user?.id,
      context?.organization?.id ?? null,
      epoch,
      isPlatformAdmin,
      canCompare,
      params,
    ],
    queryFn: async ({ signal }) => {
      const response = await getUserQuotaDates(params, isPlatformAdmin, signal)
      if (!response.success) throw new Error('Usage request failed')
      return response.data
    },
    enabled: !!user?.id && (isPlatformAdmin || !!context),
  })
  let scope: UsageScope = selectedScope
  if (!canCompare) {
    scope = isPlatformAdmin
      ? { type: 'organization' }
      : { type: 'members', userIDs: [user?.id ?? 0] }
  }
  const rows = query.isFetching || query.isError ? [] : (query.data ?? [])
  const data = selectUsageRows(rows, scope)
  return { canCompare, data, query, timeRange }
}
