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
export type OrganizationRole = 'owner' | 'admin' | 'member'
export type Organization = {
  id: number
  name: string
  slug: string
  kind: 'personal' | 'team'
  status: number
  owner_id: number
  group: string
  quota: number
  used_quota: number
  version: number
  budget_period_start: number
  budget_period_end: number
}
export type OrganizationMembership = Organization & {
  role: OrganizationRole
  spend_limit: number
}
export type OrganizationMember = {
  id: number
  org_id: number
  user_id: number
  role: OrganizationRole
  spend_limit: number
  status: number
  username: string
  display_name: string
  email: string
}
export type OrganizationContext = {
  logo?: string
  pending_transfer: boolean
  organization: Organization
  membership: OrganizationMember
  capabilities: {
    platform: Record<string, Record<string, boolean>>
    org: Record<string, Record<string, boolean>>
  }
}
export type OrganizationInvite = {
  id: number
  username: string
  invitee_id: number
  email?: string
  role: 'admin' | 'member'
  status: 'pending' | 'accepted' | 'expired' | 'revoked'
  expires_at: number
}
export type OrganizationSummary = {
  available_quota: number
  request_count: number
  quota: number
  used_quota: number
  group: string
  period_start: number
  period_end: number
  spend_limit: number
  budget_limit: number
  alert_percent: number
  member_count: number
  key_count: number
  usage: Array<{ user_id: number; used: number; reserved: number }>
  subscriptions: Array<{
    id: number
    plan_id: number
    status: string
    amount_total: number
    amount_used: number
    end_time: number
    next_reset_time: number
    allow_wallet_overflow: boolean
    upgrade_group: string
  }>
}
export type OrganizationSettings = {
  logo: string
  webhook: string
  alert_email: string
  default_spend_limit: number
  allowed_models: string[]
  alert_percent: number
  budget_limit: number
}
export type OrganizationSettingsResponse = {
  name: string
  settings: OrganizationSettings
  available_models: string[]
  transfers: Array<{ target_id: number; owner_id: number; expires_at: number }>
}
export type OrganizationOrder = {
  plan_title: string
  id: number
  org_id: number
  user_id: number
  plan_id: number
  money: number
  trade_no: string
  payment_method: string
  status: string
  create_time: number
}
export type OrganizationAudit = {
  id: number
  actor_id: number
  action: string
  object_id: string
  result: string
  created_at: number
}
export type OrganizationDeletionImpact = {
  members: number
  tokens: number
  logs: number
  orders: number
  subscriptions: number
  quota: number
  blocked: boolean
}
export type Page<T> = {
  items: T[]
  total: number
  page: number
  page_size: number
}
