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
import { useSidebarView } from '@/hooks/use-sidebar-view'
import { api } from '@/lib/http-client'
import { useAuthStore } from '@/stores/auth-store'
import { useOrganizationStore } from '@/stores/organization-store'

import { OrganizationSummary } from '../components/OrganizationSummary'
import { OrganizationSwitcher } from '../components/OrganizationSwitcher'
import { useHasTeamOrganizations } from '../context'
import { OrganizationPage } from '../index'
import { OrganizationBoundary } from '../OrganizationBoundary'
import { PlatformOrganizations } from '../PlatformOrganizations'
import type { OrganizationMembership } from '../types'

const i18n = createInstance()
await i18n
  .use(initReactI18next)
  .init({ lng: 'en', resources: { en: { translation: {} } } })
const team: OrganizationMembership = {
  id: 2,
  name: 'Design team',
  slug: 'design',
  status: 1,
  owner_id: 1,
  group: 'default',
  quota: 0,
  used_quota: 0,
  budget_period_start: 0,
  budget_period_end: 0,
  role: 'owner',
  spend_limit: 0,
}
const teamContext = {
  organization: team,
  membership: {
    id: 1,
    org_id: 2,
    user_id: 1,
    role: 'owner' as const,
    spend_limit: 0,
    status: 1,
    username: 'owner',
    display_name: '',
    email: '',
  },
  capabilities: { platform: {}, org: { 'org.settings': { write: true } } },
  pending_transfer: false,
}
const originalAdapter = api.defaults.adapter
let client: QueryClient
let listKey: unknown[]

beforeEach(() => {
  localStorage.clear()
  useAuthStore.getState().auth.setUser({ id: 1, username: 'owner', role: 100 })
  useOrganizationStore.setState(useOrganizationStore.getInitialState(), true)
  useOrganizationStore.getState().bindUser(1)
  useOrganizationStore.getState().select(null)
  const epoch = useOrganizationStore.getState().epoch
  client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  listKey = ['organizations', 1, epoch]
  client.setQueryData(listKey, [])
})

afterEach(() => {
  cleanup()
  api.defaults.adapter = originalAdapter
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
  expect(urls).toContain('/platform/organizations')
})

test('personal-only accounts do not see the organization summary panel', () => {
  render(
    <Wrapper>
      <OrganizationSummary />
    </Wrapper>
  )
  expect(screen.queryByText('Personal account')).not.toBeInTheDocument()
  expect(screen.queryByText('Organization wallet')).not.toBeInTheDocument()
  expect(screen.queryByText('My remaining limit')).not.toBeInTheDocument()
})

test('personal and team selection preserve admin navigation and show team tools only for teams', async () => {
  const { result } = renderHook(
    () => ({ sidebar: useSidebarData(), hasTeam: useHasTeamOrganizations() }),
    { wrapper: Wrapper }
  )
  await act(async () => {
    client.setQueryData(listKey, [team])
    useOrganizationStore.setState({
      activeOrgID: team.id,
      context: teamContext,
    })
  })
  await waitFor(() => expect(result.current.hasTeam).toBe(true))
  expect(result.current.sidebar.navGroups.map((group) => group.id)).toContain(
    'organization'
  )
  expect(result.current.sidebar.navGroups.map((group) => group.id)).toContain(
    'admin'
  )
  await act(async () => {
    client.setQueryData(listKey, [])
    useOrganizationStore.setState({
      activeOrgID: null,
      context: null,
    })
  })
  await waitFor(() => expect(result.current.hasTeam).toBe(false))
  expect(
    result.current.sidebar.navGroups.map((group) => group.id)
  ).not.toContain('organization')
})

test('the team switcher labels the personal section Personal and does not offer creation', async () => {
  client.setQueryData(listKey, [team])
  renderPage(OrganizationSwitcher)
  fireEvent.click(
    await screen.findByRole('button', { name: 'Switch organization' })
  )
  expect(await screen.findByRole('button', { name: 'Personal' })).toBeVisible()
  expect(screen.queryByText('personal-1')).not.toBeInTheDocument()
  expect(screen.queryByText('Personal account')).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Platform administration' })
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Return to organization' })
  ).not.toBeInTheDocument()
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

function NavigationLinks() {
  const { navGroups } = useSidebarView()
  return (
    <nav>
      {navGroups.flatMap((group) =>
        group.items.map((item) => (
          <a key={String(item.url)} href={String(item.url)}>
            {item.title}
          </a>
        ))
      )}
    </nav>
  )
}

test.each([10, 100])(
  'platform role %s sees administrator entries while a team is selected',
  async (role) => {
    useAuthStore.getState().auth.setUser({ id: 1, username: 'admin', role })
    useOrganizationStore.setState({
      activeOrgID: team.id,
      context: teamContext,
    })
    client.setQueryData(['status'], {})
    renderPage(NavigationLinks)
    expect(await screen.findByRole('link', { name: 'Channels' })).toBeVisible()
    expect(screen.getByRole('link', { name: 'Organizations' })).toBeVisible()
    expect(screen.getByRole('link', { name: 'Members' })).toBeVisible()
  }
)

test('team ownership does not grant a regular user platform navigation', async () => {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'owner', role: 1 })
  useOrganizationStore.setState({
    activeOrgID: team.id,
    context: teamContext,
  })
  client.setQueryData(['status'], {})
  renderPage(NavigationLinks)
  expect(await screen.findByRole('link', { name: 'Members' })).toBeVisible()
  expect(
    screen.queryByRole('link', { name: 'Channels' })
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('link', { name: 'Organizations' })
  ).not.toBeInTheDocument()
})

test('platform organization owners show a readable name and user ID', async () => {
  client.setQueryData(['platform-organizations', '', 1], {
    items: [
      { ...team, owner_username: 'root', owner_display_name: 'Root User' },
    ],
    total: 1,
    page: 1,
    page_size: 20,
  })
  renderPage(PlatformOrganizations)
  expect(await screen.findByText('Root User')).toBeVisible()
  expect(screen.getByText('root (#1)')).toBeVisible()
  expect(
    screen.queryByRole('columnheader', { name: 'Type' })
  ).not.toBeInTheDocument()
})

test.each([null, 99])(
  'account bootstrap works with team-only empty lists and saved selection %s',
  async (savedSelection) => {
    useOrganizationStore.getState().select(savedSelection)
    client.clear()
    const calls: string[] = []
    api.defaults.adapter = async (config) => {
      calls.push(config.url ?? '')
      expect(config.headers['X-Org-Id']).toBeUndefined()
      let data: unknown
      if (config.url === '/api/organizations') {
        data = []
      } else {
        throw new Error(`Unexpected request: ${config.url}`)
      }
      return {
        config,
        data: { success: true, data },
        status: 200,
        statusText: 'OK',
        headers: {},
      }
    }
    render(
      <Wrapper>
        <OrganizationBoundary>
          <p>Account dashboard</p>
        </OrganizationBoundary>
      </Wrapper>
    )
    expect(await screen.findByText('Account dashboard')).toBeVisible()
    expect(calls).not.toContain('/api/account/context')
    expect(calls).not.toContain('/api/org/context')
    expect(useOrganizationStore.getState().activeOrgID).toBeNull()
  }
)

test('automatic fallback after membership removal clears cached organization resources', async () => {
  useOrganizationStore.getState().select(2)
  useOrganizationStore
    .getState()
    .setContext(teamContext, useOrganizationStore.getState().epoch)
  client.setQueryData(
    ['dashboard', 'overview', 'api-keys'],
    [{ name: 'old-team-key' }]
  )
  api.defaults.adapter = async (config) => ({
    config,
    status: 200,
    statusText: 'OK',
    headers: {},
    data: { success: true, data: [] },
  })
  render(
    <OrganizationBoundary>
      <p>Personal content</p>
    </OrganizationBoundary>,
    { wrapper: Wrapper }
  )
  await screen.findByText('Personal content')
  await waitFor(() =>
    expect(useOrganizationStore.getState().activeOrgID).toBeNull()
  )
  expect(
    client.getQueryData(['dashboard', 'overview', 'api-keys'])
  ).toBeUndefined()
})
