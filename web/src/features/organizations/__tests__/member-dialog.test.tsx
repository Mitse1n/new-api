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
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import type { InternalAxiosRequestConfig } from 'axios'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/http-client'
import { useOrganizationStore } from '@/stores/organization-store'

import { MemberDialog } from '../components/MemberDialog'
import type { OrganizationMember } from '../types'

const i18n = createInstance()
await i18n
  .use(initReactI18next)
  .init({ lng: 'en', resources: { en: { translation: {} } } })
const originalAdapter = api.defaults.adapter
const requests: InternalAxiosRequestConfig[] = []
let response = {
  success: true,
  data: { id: 42 },
  message: '',
}
let client: QueryClient
const member: OrganizationMember = {
  id: 1,
  org_id: 10,
  user_id: 1,
  role: 'owner',
  status: 1,
  spend_limit: 0,
  email: 'owner@example.test',
  username: 'owner',
  display_name: 'Owner',
}

beforeEach(() => {
  localStorage.clear()
  useOrganizationStore.setState(useOrganizationStore.getInitialState(), true)
  useOrganizationStore.getState().bindUser(1)
  useOrganizationStore.getState().select(10)
  useOrganizationStore.getState().setContext(
    {
      organization: {
        id: 10,
        name: 'Review team',
        slug: 'review-team',
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
      membership: member,
      capabilities: { org: {}, platform: {} },
      pending_transfer: false,
    },
    useOrganizationStore.getState().epoch
  )
  requests.length = 0
  response = {
    success: true,
    data: { id: 42 },
    message: '',
  }
  api.defaults.adapter = async (config) => {
    requests.push(config)
    return {
      config,
      data: response,
      status: 200,
      statusText: 'OK',
      headers: {},
    }
  }
  client = new QueryClient({ defaultOptions: { mutations: { retry: false } } })
})
afterEach(() => {
  cleanup()
  client.clear()
  api.defaults.adapter = originalAdapter
  useOrganizationStore.setState(useOrganizationStore.getInitialState(), true)
  localStorage.clear()
})

function renderDialog(props: Parameters<typeof MemberDialog>[0]) {
  return render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <MemberDialog {...props} />
      </QueryClientProvider>
    </I18nextProvider>
  )
}

test('an empty invitation username shows a validation error without sending a request', async () => {
  renderDialog({ close: vi.fn() })
  expect(screen.getByRole('dialog')).toHaveAccessibleName('Invite member')
  expect(
    screen.getAllByRole('option').map((option) => option.textContent)
  ).toEqual(['Member', 'Admin'])
  fireEvent.click(screen.getByRole('button', { name: 'Confirm' }))
  await waitFor(() =>
    expect(screen.getByRole('textbox', { name: 'Username' })).toHaveAttribute(
      'aria-invalid',
      'true'
    )
  )
  expect(screen.getByText('Please enter your username')).toBeVisible()
  expect(requests).toHaveLength(0)
})

test('a successful invitation sends the username to the team and closes the dialog', async () => {
  const close = vi.fn()
  renderDialog({ close })
  fireEvent.change(screen.getByRole('textbox', { name: 'Username' }), {
    target: { value: 'new-member' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Confirm' }))
  await waitFor(() => expect(close).toHaveBeenCalledOnce())
  expect(requests).toHaveLength(1)
  expect(requests[0].headers['X-Org-Id']).toBe('10')
  expect(JSON.parse(requests[0].data)).toEqual({
    username: 'new-member',
    role: 'member',
  })
})

test('a rejected invitation preserves the username and presents the server error for correction', async () => {
  response = {
    success: false,
    data: { id: 0 },
    message: 'Team seat limit reached',
  }
  const close = vi.fn()
  renderDialog({ close })
  fireEvent.change(screen.getByRole('textbox', { name: 'Username' }), {
    target: { value: 'new-member' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Confirm' }))
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Team seat limit reached'
  )
  expect(screen.getByRole('textbox', { name: 'Username' })).toHaveValue(
    'new-member'
  )
  expect(screen.getByRole('button', { name: 'Confirm' })).toBeEnabled()
  expect(close).not.toHaveBeenCalled()
})

test('saving a member budget sends the converted quota to the budget endpoint without exposing role controls', async () => {
  const close = vi.fn()
  renderDialog({
    close,
    member: {
      ...member,
      user_id: 2,
      role: 'member',
      email: 'member@example.test',
    },
    budget: true,
  })
  expect(screen.queryByRole('combobox')).not.toBeInTheDocument()
  fireEvent.change(
    screen.getByRole('spinbutton', { name: 'Spending limit (USD)' }),
    { target: { value: '12.50' } }
  )
  fireEvent.click(screen.getByRole('button', { name: 'Confirm' }))
  await waitFor(() => expect(close).toHaveBeenCalledOnce())
  expect(requests[0].url).toBe('/api/org/members/2/budget')
  expect(JSON.parse(requests[0].data).spend_limit).toBe(6250000)
})
