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
import { api } from '@/lib/api'

import type {
  Organization,
  OrganizationAudit,
  OrganizationContext,
  OrganizationDeletionImpact,
  OrganizationInvite,
  OrganizationMember,
  OrganizationMembership,
  OrganizationOrder,
  OrganizationSettings,
  OrganizationSettingsResponse,
  OrganizationSummary,
  Page,
} from './types'

type Response<T> = { success: boolean; data: T; message?: string }

export async function listOrganizations(): Promise<OrganizationMembership[]> {
  const response = await api.get<Response<OrganizationMembership[]>>(
    '/api/organizations',
    { skipOrganizationContext: true }
  )
  if (!response.data.success) throw new Error(response.data.message)
  return response.data.data
}
export async function organizationQuery<T>(
  path: string,
  params?: Record<string, unknown>
): Promise<T> {
  const response = await api.get<Response<T>>(`/api/org/${path}`, { params })
  if (!response.data.success) throw new Error(response.data.message)
  return response.data.data
}
export async function organizationMutation<T = unknown>(
  method: 'post' | 'put' | 'delete',
  path: string,
  data?: unknown
): Promise<T> {
  const response = await api.request<Response<T>>({
    method,
    url: `/api/org/${path}`,
    data,
  })
  if (!response.data.success) throw new Error(response.data.message)
  return response.data.data
}
export async function createOrganization(data: {
  name: string
  slug: string
}): Promise<Organization> {
  const response = await api.post<Response<Organization>>(
    '/api/organizations',
    data,
    { skipOrganizationContext: true }
  )
  if (!response.data.success) throw new Error(response.data.message)
  return response.data.data
}
export const getOrganizationContext = () =>
  organizationQuery<OrganizationContext>('context')
export const getOrganizationSummary = () =>
  organizationQuery<OrganizationSummary>('summary')
export const getOrganizationMembers = () =>
  organizationQuery<OrganizationMember[]>('members')
export const getOrganizationInvites = () =>
  organizationQuery<OrganizationInvite[]>('invites')
export const getOrganizationSettings = () =>
  organizationQuery<OrganizationSettingsResponse>('settings')
export const getOrganizationOrders = (page: number) =>
  organizationQuery<Page<OrganizationOrder>>('orders', {
    p: page,
    page_size: 20,
  })
export const getOrganizationAudit = (page: number) =>
  organizationQuery<Page<OrganizationAudit>>('audit', {
    p: page,
    page_size: 20,
  })
export const updateOrganizationSettings = (data: {
  name: string
  settings: OrganizationSettings
}) => organizationMutation('put', 'settings', data)
export async function getDeletionImpact(
  orgID: number
): Promise<OrganizationDeletionImpact> {
  const response = await api.get<Response<OrganizationDeletionImpact>>(
    `/api/organizations/${orgID}/deletion-impact`
  )
  if (!response.data.success) throw new Error(response.data.message)
  return response.data.data
}
export async function changeOrganizationStatus(
  orgID: number,
  status: number,
  confirm_slug = ''
): Promise<void> {
  const response = await api.put<Response<unknown>>(
    `/api/organizations/${orgID}/status`,
    { status, confirm_slug }
  )
  if (!response.data.success) throw new Error(response.data.message)
}
export async function acceptOrganizationInvite(token: string): Promise<number> {
  const response = await api.post<Response<{ org_id: number }>>(
    '/api/organizations/invites/accept',
    { token },
    { skipOrganizationContext: true }
  )
  if (!response.data.success) throw new Error(response.data.message)
  return response.data.data.org_id
}
