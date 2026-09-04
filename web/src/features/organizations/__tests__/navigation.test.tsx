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
  renderHook,
  screen,
  waitFor,
} from '@testing-library/react'
import { createInstance } from 'i18next'
import type { ReactNode } from 'react'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, beforeEach, expect, test } from 'vitest'

import { useSidebarData } from '@/hooks/use-sidebar-data'
import { useAuthStore } from '@/stores/auth-store'
import { useOrganizationStore } from '@/stores/organization-store'

import { OrganizationSwitcher } from '../components/OrganizationSwitcher'
import { useHasTeamOrganizations } from '../context'
import { OrganizationPage } from '../index'
import type { OrganizationMembership } from '../types'

const i18n = createInstance()
await i18n
  .use(initReactI18next)
  .init({ lng: 'en', resources: { en: { translation: {} } } })
const personal: OrganizationMembership = {
  id: 1,
  name: 'Personal account',
  slug: 'personal-1',
  kind: 'personal',
  status: 1,
  owner_id: 1,
  group: 'default',
  quota: 0,
  used_quota: 0,
  version: 1,
  budget_period_start: 0,
  budget_period_end: 0,
  role: 'owner',
  spend_limit: 0,
}
const team: OrganizationMembership = {
  ...personal,
  id: 2,
  name: 'Design team',
  slug: 'design',
  kind: 'team',
}
let client: QueryClient
let listKey: unknown[]

beforeEach(() => {
  localStorage.clear()
  useAuthStore.getState().auth.setUser({ id: 1, username: 'owner', role: 100 })
  useOrganizationStore.setState(useOrganizationStore.getInitialState(), true)
  useOrganizationStore.getState().bindUser(1)
  useOrganizationStore.getState().select(1)
  const epoch = useOrganizationStore.getState().epoch
  useOrganizationStore.getState().setContext(
    {
      organization: personal,
      membership: {
        id: 1,
        org_id: 1,
        user_id: 1,
        role: 'owner',
        status: 1,
        spend_limit: 0,
        email: '',
        username: 'owner',
        display_name: 'Owner',
      },
      capabilities: {
        org: { 'org.settings': { write: true } },
        platform: { users: { read: true } },
      },
      pending_transfer: false,
    },
    epoch
  )
  client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  listKey = ['organizations', 1, epoch]
  client.setQueryData(listKey, [personal])
})

afterEach(() => {
  cleanup()
  client.clear()
  useAuthStore.getState().auth.reset()
  useOrganizationStore.setState(useOrganizationStore.getInitialState(), true)
  localStorage.clear()
})

function Wrapper(props: { children: ReactNode }) {
  return (
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        {props.children}
      </QueryClientProvider>
    </I18nextProvider>
  )
}

function renderPage(component: () => ReactNode) {
  const router = createRouter({
    routeTree: createRootRoute({ component }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  return render(
    <Wrapper>
      <RouterProvider router={router} />
    </Wrapper>
  )
}

test('personal-only accounts retain original navigation and a first-organization settings entry', () => {
  const { result } = renderHook(
    () => ({ sidebar: useSidebarData(), hasTeam: useHasTeamOrganizations() }),
    { wrapper: Wrapper }
  )
  expect(result.current.hasTeam).toBe(false)
  expect(result.current.sidebar.navGroups.map((group) => group.id)).toEqual([
    'chat',
    'general',
    'personal',
    'admin',
  ])
  const urls = result.current.sidebar.navGroups.flatMap((group) =>
    group.items.map((item) => item.url)
  )
  expect(urls).toContain('/channels')
  expect(urls).toContain('/wallet')
  expect(urls).toContain('/organization/settings')
  expect(urls).not.toContain('/platform/organizations')
})

test('joining and leaving the last team updates organization navigation', async () => {
  const { result } = renderHook(
    () => ({ sidebar: useSidebarData(), hasTeam: useHasTeamOrganizations() }),
    { wrapper: Wrapper }
  )
  await act(async () => {
    client.setQueryData(listKey, [personal, team])
  })
  await waitFor(() => expect(result.current.hasTeam).toBe(true))
  expect(result.current.sidebar.navGroups.map((group) => group.id)).toContain(
    'organization'
  )
  expect(
    result.current.sidebar.navGroups.map((group) => group.id)
  ).not.toContain('admin')
  await act(async () => {
    client.setQueryData(listKey, [personal])
  })
  await waitFor(() => expect(result.current.hasTeam).toBe(false))
  expect(
    result.current.sidebar.navGroups.map((group) => group.id)
  ).not.toContain('organization')
})

test('the team switcher labels the personal section Personal and does not offer creation', async () => {
  client.setQueryData(listKey, [personal, team])
  renderPage(OrganizationSwitcher)
  fireEvent.click(
    await screen.findByRole('button', { name: 'Switch organization' })
  )
  expect(await screen.findByText('Personal', { exact: true })).toBeVisible()
  expect(screen.queryByText('Personal organizations')).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Create organization' })
  ).not.toBeInTheDocument()
})

test('personal settings open the first-organization creation form without team billing or settings', async () => {
  renderPage(() => <OrganizationPage section='settings' />)
  fireEvent.click(
    await screen.findByRole('button', { name: 'Create organization' })
  )
  expect(
    await screen.findByRole('dialog', { name: 'Create organization' })
  ).toBeVisible()
  expect(
    screen.getByRole('textbox', { name: 'Organization name' })
  ).toBeVisible()
  expect(
    screen.queryByRole('button', { name: 'Save changes' })
  ).not.toBeInTheDocument()
  expect(screen.queryByText('Danger zone')).not.toBeInTheDocument()
})
