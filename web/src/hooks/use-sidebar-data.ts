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
import {
  Activity,
  Box,
  CreditCard,
  FileText,
  FlaskConical,
  Key,
  LayoutDashboard,
  ListTodo,
  MessageSquare,
  PlugZap,
  Radio,
  ServerCog,
  Settings,
  Ticket,
  User,
  Users,
  Wallet,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import type { SidebarData } from '@/components/layout/types'
import { useHasTeamOrganizations } from '@/features/organizations/context'
import { ROLE } from '@/lib/roles'
import { useOrganizationStore } from '@/stores/organization-store'

/**
 * Root navigation groups for the application sidebar.
 *
 * These are shown when the URL does not match any nested sidebar view
 * registered in `layout/lib/sidebar-view-registry.ts`.
 */
export function useSidebarData(): SidebarData {
  const { t } = useTranslation()
  const platform = useOrganizationStore((state) => state.platform)
  const hasTeamOrganizations = useHasTeamOrganizations()
  const capabilities = useOrganizationStore(
    (state) => state.context?.capabilities.org
  )

  const data: SidebarData = {
    navGroups: [
      {
        id: 'chat',
        title: t('Chat'),
        items: [
          {
            title: t('Playground'),
            url: '/playground',
            icon: FlaskConical,
          },
          {
            title: t('Chat'),
            icon: MessageSquare,
            type: 'chat-presets',
          },
        ],
      },
      {
        id: 'general',
        title: t('General'),
        items: [
          {
            title: t('Overview'),
            url: '/dashboard/overview',
            icon: Activity,
          },
          {
            title: t('Dashboard'),
            url: '/dashboard/models',
            icon: LayoutDashboard,
          },
          {
            title: t('API Keys'),
            url: '/keys',
            icon: Key,
          },
          {
            title: t('Usage Logs'),
            url: '/usage-logs/common',
            icon: FileText,
          },
          {
            title: t('Task Logs'),
            url: '/usage-logs/task',
            activeUrls: ['/usage-logs/drawing'],
            configUrls: ['/usage-logs/drawing', '/usage-logs/task'],
            icon: ListTodo,
          },
        ],
      },
      {
        id: 'personal',
        title: t('Personal'),
        items: [
          {
            title: t('Wallet'),
            url: '/wallet',
            icon: Wallet,
          },
          {
            title: t('Profile'),
            url: '/profile',
            icon: User,
          },
        ],
      },
      {
        id: 'admin',
        title: t('Admin'),
        items: [
          {
            title: t('Organizations'),
            url: '/platform/organizations',
            icon: Users,
            requiredRole: ROLE.ADMIN,
          },
          {
            title: t('Channels'),
            url: '/channels',
            icon: Radio,
          },
          {
            title: t('Models'),
            url: '/models/metadata',
            icon: Box,
          },
          {
            title: t('Users'),
            url: '/users',
            icon: Users,
          },
          {
            title: t('Redemption Codes'),
            url: '/redemption-codes',
            icon: Ticket,
          },
          {
            title: t('Subscriptions'),
            url: '/subscriptions',
            icon: CreditCard,
          },
          {
            title: t('System Info'),
            url: '/system-info',
            icon: ServerCog,
            requiredRole: ROLE.SUPER_ADMIN,
          },
          {
            title: t('Task Plugins'),
            url: '/task-plugins',
            icon: PlugZap,
            requiredRole: ROLE.SUPER_ADMIN,
          },
          {
            title: t('System Settings'),
            url: '/system-settings/site',
            activeUrls: ['/system-settings'],
            icon: Settings,
          },
        ],
      },
    ],
  }
  if (!hasTeamOrganizations) {
    // Personal-only accounts keep the original navigation and admin entries.
    data.navGroups = data.navGroups.map((group) => ({
      ...group,
      items: group.items.filter(
        (item) => item.url !== '/platform/organizations'
      ),
    }))
    data.navGroups
      .find((group) => group.id === 'personal')
      ?.items.push({
        title: t('Organization settings'),
        url: '/organization/settings',
        icon: Settings,
      })
  } else if (!platform) {
    data.navGroups = data.navGroups
      .filter((group) => group.id !== 'admin')
      .map((group) => ({
        ...group,
        items: group.items.filter(
          (item) =>
            item.url !== '/wallet' || capabilities?.['org.billing']?.read
        ),
      }))
    data.navGroups.splice(2, 0, {
      id: 'organization',
      title: t('Organization'),
      items: [
        { title: t('Members'), url: '/organization/members', icon: Users },
        {
          title: t('Billing & budgets'),
          url: '/organization/billing',
          icon: Wallet,
        },
        ...(capabilities?.['org.subscription']?.purchase
          ? [
              {
                title: t('Plans & orders'),
                url: '/organization/plans',
                icon: CreditCard,
              },
            ]
          : []),
        ...(capabilities?.['org.settings']?.write
          ? [
              {
                title: t('Organization settings'),
                url: '/organization/settings',
                icon: Settings,
              },
            ]
          : []),
        ...(capabilities?.['org.usage']?.read_all
          ? [
              {
                title: t('Organization audit'),
                url: '/organization/audit',
                icon: FileText,
              },
            ]
          : []),
      ],
    })
  }
  return data
}
