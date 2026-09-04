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
import { api, type ApiRequestConfig } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'
import { useOrganizationStore } from '@/stores/organization-store'

import { buildQueryParams } from './lib/query-params'
import { parseTaskArtifactsResponse } from './lib/task-artifacts'
import type {
  GetLogsParams,
  GetLogsResponse,
  GetLogStatsParams,
  GetLogStatsResponse,
  GetMidjourneyLogsParams,
  GetTaskLogsParams,
  TaskArtifactsResponse,
  UserInfo,
} from './types'

// ============================================================================
// Generic API Helpers
// ============================================================================

function buildApiPath(
  endpoint: string,
  isAdmin: boolean,
  platform: boolean
): string {
  const state = useOrganizationStore.getState()
  if (state.context && !platform) {
    return endpoint === '/api/log' ? '/api/org/logs' : `${endpoint}/self`
  }
  return isAdmin ? endpoint : `${endpoint}/self`
}

async function fetchLogs<T>(
  endpoint: string,
  params: T,
  isAdmin: boolean,
  platform = false
): Promise<GetLogsResponse> {
  const state = useOrganizationStore.getState()
  const paramRecord = params as unknown as Record<string, unknown>
  if (state.context && !platform && !isAdmin) {
    paramRecord.user_id = useAuthStore.getState().auth.user?.id
  }
  const queryParams = buildQueryParams({
    p: paramRecord.p || 1,
    page_size: paramRecord.page_size || 20,
    ...params,
  })
  const path = buildApiPath(endpoint, isAdmin, platform)
  const res = await api.get(`${path}?${queryParams}`, {
    skipOrganizationContext: platform,
  })
  return res.data
}

async function fetchLogStats<T>(
  endpoint: string,
  params: T,
  isAdmin: boolean,
  platform = false
): Promise<GetLogStatsResponse> {
  const paramRecord = params as unknown as Record<string, unknown>
  const state = useOrganizationStore.getState()
  if (state.context && !platform && !isAdmin) {
    paramRecord.user_id = useAuthStore.getState().auth.user?.id
  }
  const queryParams = buildQueryParams(paramRecord)
  const path = buildApiPath(endpoint, isAdmin, platform)
  const res = await api.get(`${path}/stat?${queryParams}`, {
    skipOrganizationContext: platform,
  })
  return res.data
}

// ============================================================================
// Common Log APIs
// ============================================================================

export const getAllLogs = (params: GetLogsParams = {}, platform = false) =>
  fetchLogs('/api/log', params, true, platform)

export const getUserLogs = (
  params: Omit<GetLogsParams, 'username' | 'channel'> = {},
  platform = false
) => fetchLogs('/api/log', params, false, platform)

export const getLogStats = (params: GetLogStatsParams = {}, platform = false) =>
  fetchLogStats('/api/log', params, true, platform)

export const getUserLogStats = (
  params: Omit<GetLogStatsParams, 'username' | 'channel'> = {},
  platform = false
) => fetchLogStats('/api/log', params, false, platform)

export async function getUserInfo(
  userId: number
): Promise<{ success: boolean; message?: string; data?: UserInfo }> {
  const res = await api.get(`/api/user/${userId}`)
  return res.data
}

// ============================================================================
// MjProxy (Drawing) Logs API
// ============================================================================

export const getAllMidjourneyLogs = (
  params: GetMidjourneyLogsParams,
  platform = false
) => fetchLogs('/api/mj', params, true, platform)

export const getUserMidjourneyLogs = (
  params: GetMidjourneyLogsParams,
  platform = false
) => fetchLogs('/api/mj', params, false, platform)

// ============================================================================
// Task Logs API
// ============================================================================

export const getAllTaskLogs = (params: GetTaskLogsParams, platform = false) =>
  fetchLogs('/api/task', params, true, platform)

export const getUserTaskLogs = (params: GetTaskLogsParams, platform = false) =>
  fetchLogs('/api/task', params, false, platform)

const taskArtifactRequestConfig = {
  skipBusinessError: true,
  skipErrorHandler: true,
} satisfies ApiRequestConfig

export async function getTaskArtifacts(taskId: string) {
  const response = await api.get<TaskArtifactsResponse>(
    `/api/task/${encodeURIComponent(taskId)}/artifacts`,
    taskArtifactRequestConfig
  )
  return parseTaskArtifactsResponse(response.data)
}
