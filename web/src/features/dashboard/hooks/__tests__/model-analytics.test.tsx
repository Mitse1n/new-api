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
import { act, cleanup, renderHook, waitFor } from '@testing-library/react'
import type { AxiosResponse, InternalAxiosRequestConfig } from 'axios'
import type { PropsWithChildren } from 'react'
import { afterEach, beforeEach, expect, test } from 'vitest'

import { calculateDashboardStats } from '@/features/dashboard/lib/stats'
import type { UsageScope } from '@/features/dashboard/lib/usage-scope'
import type { QuotaDataItem } from '@/features/dashboard/types'
import { PlatformViewContext } from '@/features/organizations/platform-view'
import type { OrganizationContext } from '@/features/organizations/types'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'
import { useOrganizationStore } from '@/stores/organization-store'

import { useModelAnalytics } from '../use-model-analytics'

const originalAdapter = api.defaults.adapter
const pending: Array<{
  config: InternalAxiosRequestConfig
  resolve: (value: AxiosResponse) => void
}> = []
const filters = {
  start_timestamp: new Date('2026-09-01T00:00:00Z'),
  end_timestamp: new Date('2026-09-02T00:00:00Z'),
}
const scope: UsageScope = { type: 'organization' }
const rows: QuotaDataItem[] = [
  {
    user_id: 1,
    model_name: 'mock-openai',
    created_at: 1788220800,
    count: 2,
    quota: 20,
    token_used: 100,
  },
  {
    user_id: 2,
    model_name: 'mock-claude',
    created_at: 1788220800,
    count: 3,
    quota: 30,
    token_used: 200,
  },
]
let client: QueryClient

function setOrganization(id: number, canReadAll = true) {
  useOrganizationStore.getState().select(id)
  const context: OrganizationContext = {
    organization: {
      id,
      name: `Team ${id}`,
      slug: `team-${id}`,
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
      org_id: id,
      user_id: 1,
      role: 'owner',
      spend_limit: 0,
      status: 1,
      username: 'viewer',
      display_name: 'Viewer',
      email: '',
    },
    pending_transfer: false,
    capabilities: {
      platform: {},
      org: { 'org.usage': { read_all: canReadAll } },
    },
  }
  useOrganizationStore
    .getState()
    .setContext(context, useOrganizationStore.getState().epoch)
}

function Wrapper(props: PropsWithChildren) {
  return (
    <QueryClientProvider client={client}>{props.children}</QueryClientProvider>
  )
}

function respond(index: number, data: QuotaDataItem[], success = true) {
  const request = pending[index]
  request.resolve({
    config: request.config,
    data: { success, data },
    status: 200,
    statusText: 'OK',
    headers: {},
  })
}

beforeEach(() => {
  localStorage.clear()
  pending.length = 0
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  useOrganizationStore.setState(useOrganizationStore.getInitialState(), true)
  useOrganizationStore.getState().bindUser(1)
  useAuthStore.getState().auth.setUser({ id: 1, username: 'viewer', role: 100 })
  setOrganization(10)
  api.defaults.adapter = (config) =>
    new Promise((resolve) => pending.push({ config, resolve }))
})

afterEach(() => {
  cleanup()
  client.clear()
  api.defaults.adapter = originalAdapter
  useOrganizationStore.setState(useOrganizationStore.getInitialState(), true)
  useAuthStore.getState().auth.reset()
  localStorage.clear()
})

test('organization totals and multi-member selections aggregate only matching members, without duplicating rows', async () => {
  const { result, rerender } = renderHook(
    ({ selection }: { selection: UsageScope }) =>
      useModelAnalytics(filters, selection),
    {
      initialProps: { selection: scope as UsageScope },
      wrapper: Wrapper,
    }
  )
  await waitFor(() => expect(pending).toHaveLength(1))
  expect(pending[0].config.url).toBe('/api/data/self')
  expect(pending[0].config.headers['X-Org-Id']).toBe('10')
  act(() => respond(0, rows))
  await waitFor(() => expect(result.current.query.isSuccess).toBe(true))
  expect(calculateDashboardStats(result.current.data)).toEqual({
    totalCount: 5,
    totalQuota: 50,
    totalTokens: 300,
  })
  rerender({ selection: { type: 'members', userIDs: [2] } })
  expect(calculateDashboardStats(result.current.data)).toEqual({
    totalCount: 3,
    totalQuota: 30,
    totalTokens: 200,
  })
  rerender({ selection: { type: 'members', userIDs: [1, 2, 2] } })
  expect(calculateDashboardStats(result.current.data)).toEqual({
    totalCount: 5,
    totalQuota: 50,
    totalTokens: 300,
  })
  rerender({ selection: { type: 'members', userIDs: [3] } })
  expect(calculateDashboardStats(result.current.data)).toEqual({
    totalCount: 0,
    totalQuota: 0,
    totalTokens: 0,
  })
  expect(pending).toHaveLength(1)
})

test('members remain restricted to self even with a platform admin role and forged scope selection', async () => {
  setOrganization(10, false)
  const { result } = renderHook(() => useModelAnalytics(filters, scope), {
    wrapper: Wrapper,
  })
  await waitFor(() => expect(pending).toHaveLength(1))
  act(() => respond(0, rows))
  await waitFor(() => expect(result.current.query.isSuccess).toBe(true))
  expect(result.current.canCompare).toBe(false)
  expect(result.current.data).toEqual([rows[0]])
})

test('switching organization ignores a late previous response and displays only the new organization', async () => {
  const { result } = renderHook(() => useModelAnalytics(filters, scope), {
    wrapper: Wrapper,
  })
  await waitFor(() => expect(pending).toHaveLength(1))
  act(() => setOrganization(20))
  await waitFor(() => expect(pending).toHaveLength(2))
  expect(pending[1].config.headers['X-Org-Id']).toBe('20')
  act(() => respond(0, rows))
  expect(result.current.data).toEqual([])
  const newRows = [{ ...rows[0], count: 7 }]
  act(() => respond(1, newRows))
  await waitFor(() => expect(result.current.query.isSuccess).toBe(true))
  expect(result.current.data).toEqual(newRows)
})

test('failed usage responses show an error instead of successful zero statistics', async () => {
  const { result } = renderHook(() => useModelAnalytics(filters, scope), {
    wrapper: Wrapper,
  })
  await waitFor(() => expect(pending).toHaveLength(1))
  act(() => respond(0, [], false))
  await waitFor(() => expect(result.current.query.isError).toBe(true))
  expect(result.current.data).toEqual([])
})

test('platform analytics still query global data independently of the selected organization', async () => {
  const { result } = renderHook(() => useModelAnalytics(filters, scope), {
    wrapper: (props: PropsWithChildren) => (
      <Wrapper>
        <PlatformViewContext value>{props.children}</PlatformViewContext>
      </Wrapper>
    ),
  })
  await waitFor(() => expect(pending).toHaveLength(1))
  expect(pending[0].config.url).toBe('/api/data')
  expect(pending[0].config.headers['X-Org-Id']).toBeUndefined()
  act(() => respond(0, rows))
  await waitFor(() => expect(result.current.query.isSuccess).toBe(true))
  expect(result.current.canCompare).toBe(false)
  expect(result.current.data).toEqual(rows)
})
