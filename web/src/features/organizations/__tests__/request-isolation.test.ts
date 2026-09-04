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
import axios, {
  type AxiosAdapter,
  type AxiosResponse,
  type InternalAxiosRequestConfig,
} from 'axios'
import { afterEach, beforeEach, expect, test } from 'vitest'

import { api } from '@/lib/http-client'
import { useOrganizationStore } from '@/stores/organization-store'

const originalAdapter = api.defaults.adapter
const responses: Array<{
  config: InternalAxiosRequestConfig
  resolve: (value: AxiosResponse) => void
}> = []
const adapter: AxiosAdapter = (config) =>
  new Promise((resolve) => {
    responses.push({ config, resolve })
  })

beforeEach(() => {
  localStorage.clear()
  responses.length = 0
  useOrganizationStore.setState(useOrganizationStore.getInitialState(), true)
  useOrganizationStore.getState().bindUser(1)
  useOrganizationStore.getState().select(10)
  api.defaults.adapter = adapter
})
afterEach(() => {
  api.defaults.adapter = originalAdapter
  useOrganizationStore.setState(useOrganizationStore.getInitialState(), true)
  localStorage.clear()
})

test('switching organizations discards the previous response and starts a separately scoped request', async () => {
  const oldRequest = api
    .get('/api/org/summary')
    .catch((error: unknown) => error)
  useOrganizationStore.getState().select(20)
  const currentRequest = api.get('/api/org/summary')
  expect(responses).toHaveLength(2)
  expect(responses[0].config.headers['X-Org-Id']).toBe('10')
  expect(responses[1].config.headers['X-Org-Id']).toBe('20')
  responses[0].resolve({
    config: responses[0].config,
    data: { success: true, data: { quota: 100 } },
    status: 200,
    statusText: 'OK',
    headers: {},
  })
  responses[1].resolve({
    config: responses[1].config,
    data: { success: true, data: { quota: 200 } },
    status: 200,
    statusText: 'OK',
    headers: {},
  })
  expect(axios.isCancel(await oldRequest)).toBe(true)
  expect((await currentRequest).data.data.quota).toBe(200)
})

test('a submitted mutation remains bound to its original organization when selection changes', async () => {
  const request = api
    .post('/api/token/', { name: 'team key' })
    .catch((error: unknown) => error)
  useOrganizationStore.getState().select(20)
  expect(responses).toHaveLength(1)
  expect(responses[0].config.headers['X-Org-Id']).toBe('10')
  responses[0].resolve({
    config: responses[0].config,
    data: { success: true },
    status: 200,
    statusText: 'OK',
    headers: {},
  })
  expect(axios.isCancel(await request)).toBe(true)
})

test('signing in as another identity never restores the previous identity’s selected organization', () => {
  useOrganizationStore.getState().select(10)
  useOrganizationStore.getState().bindUser(2)
  expect(useOrganizationStore.getState().activeOrgID).toBeNull()
  expect(useOrganizationStore.getState().context).toBeNull()
  useOrganizationStore.getState().select(20)
  useOrganizationStore.getState().bindUser(1)
  expect(useOrganizationStore.getState().activeOrgID).toBe(10)
  expect(useOrganizationStore.getState().context).toBeNull()
})
