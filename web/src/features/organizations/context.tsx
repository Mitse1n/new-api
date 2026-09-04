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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate, useRouterState } from '@tanstack/react-router'

import { useAuthStore } from '@/stores/auth-store'
import { useOrganizationStore } from '@/stores/organization-store'

import { listOrganizations } from './api'

export function useHasTeamOrganizations() {
  const userID = useAuthStore((state) => state.auth.user?.id)
  const epoch = useOrganizationStore((state) => state.epoch)
  const organizations = useQuery({
    queryKey: ['organizations', userID, epoch],
    queryFn: listOrganizations,
    enabled: !!userID,
    select: (items) =>
      items.some((organization) => organization.kind === 'team'),
  })
  return organizations.data ?? false
}

export function useOrganization() {
  const context = useOrganizationStore((state) => state.context)
  if (!context) throw new Error('Organization context is not ready')
  return context
}

export function useSwitchOrganization() {
  const client = useQueryClient()
  const navigate = useNavigate()
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  })
  return (orgID: number) => {
    void client.cancelQueries()
    client.removeQueries()
    useOrganizationStore.getState().select(orgID)
    if (pathname.startsWith('/platform/')) return
    const organizationSection = pathname.match(
      /^\/organization\/(members|billing|plans|settings|audit)$/
    )?.[1]
    const dashboardSection = pathname.match(/^\/dashboard\/([^/]+)$/)?.[1]
    if (organizationSection) {
      void navigate({
        to: '/organization/$section',
        params: { section: organizationSection },
      })
      return
    }
    if (dashboardSection) {
      void navigate({
        to: '/dashboard/$section',
        params: { section: dashboardSection },
      })
      return
    }
    if (pathname === '/keys') {
      void navigate({ to: '/keys', search: {} })
      return
    }
    if (pathname === '/wallet') {
      void navigate({ to: '/wallet', search: {} })
      return
    }
    if (pathname.startsWith('/usage-logs/')) {
      void navigate({
        to: '/usage-logs/$section',
        params: { section: pathname.split('/')[2] },
        search: {},
      })
      return
    }
    void navigate({
      to: '/dashboard/$section',
      params: { section: 'overview' },
    })
  }
}
