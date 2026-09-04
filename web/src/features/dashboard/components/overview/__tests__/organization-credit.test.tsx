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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { cleanup, render, screen } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, expect, test } from 'vitest'

import type { OrganizationContext } from '@/features/organizations/types'
import { api } from '@/lib/http-client'
import { useOrganizationStore } from '@/stores/organization-store'

import { SummaryCards } from '../summary-cards'

const originalAdapter = api.defaults.adapter
const client = new QueryClient({
  defaultOptions: { queries: { retry: false, staleTime: Infinity } },
})
const i18n = createInstance()
await i18n
  .use(initReactI18next)
  .init({ lng: 'en', resources: { en: { translation: {} } } })

afterEach(() => {
  cleanup()
  client.clear()
  api.defaults.adapter = originalAdapter
  useOrganizationStore.setState(useOrganizationStore.getInitialState(), true)
  localStorage.clear()
})

test('a team with subscription credit and an empty wallet shows usable credit without a depleted warning', async () => {
  localStorage.clear()
  useOrganizationStore.setState(useOrganizationStore.getInitialState(), true)
  useOrganizationStore.getState().bindUser(1)
  useOrganizationStore.getState().select(10)
  const context: OrganizationContext = {
    organization: {
      id: 10,
      name: 'Subscription team',
      slug: 'subscription-team',
      kind: 'team',
      status: 1,
      owner_id: 1,
      group: 'default',
      quota: 0,
      used_quota: 0,
      version: 1,
      budget_period_start: 0,
      budget_period_end: 0,
    },
    membership: {
      id: 1,
      org_id: 10,
      user_id: 1,
      role: 'owner',
      status: 1,
      spend_limit: 0,
      email: '',
      username: 'owner',
      display_name: 'Owner',
    },
    capabilities: { org: {}, platform: {} },
    pending_transfer: false,
  }
  useOrganizationStore
    .getState()
    .setContext(context, useOrganizationStore.getState().epoch)
  client.setQueryData(['organization-summary', 10], {
    quota: 0,
    available_quota: 500000,
    used_quota: 0,
    request_count: 0,
  })
  client.setQueryData(['status'], { display_in_currency: true })
  api.defaults.adapter = async (config) => ({
    config,
    status: 200,
    statusText: 'OK',
    headers: {},
    data: { success: true, data: [] },
  })
  const route = createRootRoute({ component: SummaryCards })
  const router = createRouter({
    routeTree: route,
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    </I18nextProvider>
  )
  expect(await screen.findByText('$1')).toBeVisible()
  expect(screen.queryByText('Balance depleted')).not.toBeInTheDocument()
  expect(screen.getByText('No recent usage')).toBeVisible()
})
