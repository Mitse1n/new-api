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
import type { InternalAxiosRequestConfig } from 'axios'
import { afterEach, beforeEach, expect, test } from 'vitest'

import { getUserQuotaDates } from '@/features/dashboard/api'
import { api } from '@/lib/http-client'
import { useAuthStore } from '@/stores/auth-store'
import { useOrganizationStore } from '@/stores/organization-store'

import {
  getAllLogs,
  getUserLogs,
  getAllTaskLogs,
  getAllMidjourneyLogs,
  getLogStats,
} from '../api'

const originalAdapter = api.defaults.adapter
const requests: InternalAxiosRequestConfig[] = []

beforeEach(() => {
  localStorage.clear()
  useAuthStore.getState().auth.setUser({ id: 1, username: 'admin', role: 100 })
  useOrganizationStore.setState(useOrganizationStore.getInitialState(), true)
  useOrganizationStore.getState().bindUser(1)
  useOrganizationStore.getState().select(10)
  useOrganizationStore.getState().setContext(
    {
      organization: {
        id: 10,
        name: 'Team',
        slug: 'team',
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
        username: 'admin',
        display_name: '',
        email: '',
      },
      capabilities: { org: { 'org.usage': { read_all: true } }, platform: {} },
      pending_transfer: false,
    },
    useOrganizationStore.getState().epoch
  )
  requests.length = 0
  api.defaults.adapter = async (config) => {
    requests.push(config)
    return {
      config,
      status: 200,
      statusText: 'OK',
      headers: {},
      data: { success: true, data: [] },
    }
  }
})

afterEach(() => {
  api.defaults.adapter = originalAdapter
  useAuthStore.getState().auth.reset()
  useOrganizationStore.setState(useOrganizationStore.getInitialState(), true)
  localStorage.clear()
})

test('organization-wide logs stay scoped even when the logged-in account is a super administrator', async () => {
  await getAllLogs({})
  const url = new URL(requests[0].url ?? '', 'https://example.test')
  expect(url.pathname).toBe('/api/org/logs')
  expect(url.searchParams.has('user_id')).toBe(false)
  expect(requests[0].headers['X-Org-Id']).toBe('10')
})

test('personal log view restricts requests to the logged-in user', async () => {
  await getUserLogs({})
  const url = new URL(requests[0].url ?? '', 'https://example.test')
  expect(url.pathname).toBe('/api/org/logs')
  expect(url.searchParams.get('user_id')).toBe('1')
  expect(requests[0].headers['X-Org-Id']).toBe('10')
})

test.each([
  [getAllLogs, '/api/log'],
  [getAllTaskLogs, '/api/task'],
  [getAllMidjourneyLogs, '/api/mj'],
  [getLogStats, '/api/log/stat'],
] as const)(
  'platform log request uses its global endpoint %s independently of organization selection',
  async (request, path) => {
    await request({}, true)
    expect(
      new URL(requests[0].url ?? '', 'https://example.test').pathname
    ).toBe(path)
    expect(requests[0].headers['X-Org-Id']).toBeUndefined()
    useOrganizationStore.getState().select(20)
    await request({}, true)
    expect(
      new URL(requests[1].url ?? '', 'https://example.test').pathname
    ).toBe(path)
    expect(requests[1].headers['X-Org-Id']).toBeUndefined()
  }
)

test('personal and platform dashboard requests use distinct endpoints and scopes', async () => {
  const range = { start_timestamp: 0, end_timestamp: 1 }
  await getUserQuotaDates(range)
  await getUserQuotaDates(range, true)
  expect(requests[0].url).toBe('/api/data/self')
  expect(requests[0].headers['X-Org-Id']).toBe('10')
  expect(requests[1].url).toBe('/api/data')
  expect(requests[1].headers['X-Org-Id']).toBeUndefined()
})
