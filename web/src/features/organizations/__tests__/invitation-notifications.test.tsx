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
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { AxiosError, type InternalAxiosRequestConfig } from 'axios'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { NotificationPopover } from '@/components/notification-popover'
import { api } from '@/lib/http-client'
import { useAuthStore } from '@/stores/auth-store'
import { useOrganizationStore } from '@/stores/organization-store'

import type { IncomingOrganizationInvite } from '../types'

const i18n = createInstance()
await i18n
  .use(initReactI18next)
  .init({ lng: 'en', resources: { en: { translation: {} } } })
const originalAdapter = api.defaults.adapter
const invitation: IncomingOrganizationInvite = {
  id: 42,
  org_id: 20,
  organization_name: 'Team Inbox',
  inviter_username: 'owner',
  role: 'member',
  expires_at: 2000000000,
}
let pending: IncomingOrganizationInvite[]
let requests: InternalAxiosRequestConfig[]
let client: QueryClient
let rejectResponse: boolean
const close = vi.fn()

beforeEach(() => {
  localStorage.clear()
  useAuthStore
    .getState()
    .auth.setUser({ id: 2, username: 'recipient', email: '', role: 1 })
  useOrganizationStore.setState(useOrganizationStore.getInitialState(), true)
  useOrganizationStore.getState().bindUser(2)
  useOrganizationStore.getState().select(10)
  pending = [invitation]
  requests = []
  rejectResponse = false
  close.mockReset()
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  api.defaults.adapter = async (config) => {
    requests.push(config)
    if (config.method === 'get') {
      return {
        config,
        data: { success: true, data: pending },
        status: 200,
        statusText: 'OK',
        headers: {},
      }
    }
    if (rejectResponse) {
      throw new AxiosError('Request failed', undefined, config, undefined, {
        config,
        data: { success: false, code: 'ORG_INVITE' },
        status: 400,
        statusText: 'Bad Request',
        headers: {},
      })
    }
    pending = []
    return {
      config,
      data: { success: true, data: { org_id: 20 } },
      status: 200,
      statusText: 'OK',
      headers: {},
    }
  }
})
afterEach(() => {
  cleanup()
  client.clear()
  api.defaults.adapter = originalAdapter
  useAuthStore.getState().auth.reset()
  useOrganizationStore.setState(useOrganizationStore.getInitialState(), true)
  localStorage.clear()
})
function renderNotifications() {
  const router = createRouter({
    routeTree: createRootRoute({
      component: () => (
        <NotificationPopover
          open
          onOpenChange={close}
          unreadCount={0}
          activeTab='notice'
          onTabChange={() => {}}
          notice=''
          announcements={[]}
          loading={false}
        />
      ),
    }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    </I18nextProvider>
  )
}

test('a recipient without an email sees an account-scoped invitation and can accept directly', async () => {
  renderNotifications()
  expect(await screen.findByText('Team Inbox')).toBeVisible()
  expect(screen.getByText('owner invited you to join as Member.')).toBeVisible()
  expect(screen.getByText('1')).toBeVisible()
  expect(requests[0].url).toBe('/api/organizations/invites')
  expect(requests[0].headers['X-Org-Id']).toBeUndefined()
  fireEvent.click(screen.getByRole('button', { name: 'Accept invitation' }))
  await waitFor(() =>
    expect(useOrganizationStore.getState().activeOrgID).toBe(20)
  )
  const request = requests.find((item) => item.method === 'post')
  expect(request?.url).toBe('/api/organizations/invites/42/accept')
  expect(request?.headers['X-Org-Id']).toBeUndefined()
  expect(JSON.parse(request?.data ?? 'null')).toEqual({})
  expect(close).toHaveBeenCalledWith(false)
})

test('declining removes the invitation without switching the current organization', async () => {
  renderNotifications()
  fireEvent.click(
    await screen.findByRole('button', { name: 'Decline invitation' })
  )
  await waitFor(() =>
    expect(screen.queryByText('Team Inbox')).not.toBeInTheDocument()
  )
  expect(requests.find((item) => item.method === 'post')?.url).toBe(
    '/api/organizations/invites/42/decline'
  )
  expect(useOrganizationStore.getState().activeOrgID).toBe(10)
  expect(close).not.toHaveBeenCalled()
})

test('a rejected response shows the invitation error without changing organizations', async () => {
  rejectResponse = true
  renderNotifications()
  fireEvent.click(
    await screen.findByRole('button', { name: 'Accept invitation' })
  )
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Invitation unavailable or identity does not match'
  )
  expect(useOrganizationStore.getState().activeOrgID).toBe(10)
  expect(close).not.toHaveBeenCalled()
})

test('switching accounts does not display the previous recipient invitation', async () => {
  renderNotifications()
  expect(await screen.findByText('Team Inbox')).toBeVisible()
  pending = []
  await act(async () => {
    useAuthStore.getState().auth.setUser({ id: 3, username: 'other', role: 1 })
  })
  await waitFor(() =>
    expect(screen.queryByText('Team Inbox')).not.toBeInTheDocument()
  )
})
