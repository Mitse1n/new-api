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
import axios, { CanceledError, type AxiosRequestConfig } from 'axios'
import { t } from 'i18next'
import { toast } from 'sonner'

import {
  applyAuthRotation,
  clearAuthentication,
  refreshAuthentication,
} from '@/lib/auth-session'
import { getServerErrorMessageKey } from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'
import { useOrganizationStore } from '@/stores/organization-store'

declare module 'axios' {
  export interface AxiosRequestConfig {
    skipOrganizationContext?: boolean
    organizationEpoch?: number
    organizationID?: number | null
    skipBusinessError?: boolean
    skipErrorHandler?: boolean
    disableDuplicate?: boolean
    skipAuthRefresh?: boolean
    authRetry?: boolean
    acceptAuthRotation?: boolean
  }
}

export type ApiRequestConfig = AxiosRequestConfig

export const api = axios.create({
  baseURL: '',
  withCredentials: true,
  headers: {
    'Cache-Control': 'no-store',
  },
})

const inFlightGet = new Map<string, Promise<unknown>>()
const originalGet = api.get.bind(api)

api.get = ((url: string, config: ApiRequestConfig = {}) => {
  if (config.disableDuplicate) return originalGet(url, config)

  const params = config.params ? JSON.stringify(config.params) : '{}'
  const sessionSID = useAuthStore.getState().auth.session?.sid || 'anonymous'
  const scope = useOrganizationStore.getState()
  const key = `${sessionSID}:${scope.epoch}:${scope.activeOrgID}:${url}?${params}`
  const existingRequest = inFlightGet.get(key)
  if (existingRequest) return existingRequest

  const request = originalGet(url, config).finally(() => {
    inFlightGet.delete(key)
  })
  inFlightGet.set(key, request)
  return request
}) as typeof api.get

function redirectToSignIn(): void {
  if (
    typeof window !== 'undefined' &&
    window.location.pathname !== '/sign-in'
  ) {
    window.location.replace('/sign-in')
  }
}

api.interceptors.response.use(
  (response) => {
    if (
      response.config.organizationEpoch !== undefined &&
      response.config.organizationEpoch !==
        useOrganizationStore.getState().epoch
    ) {
      throw new CanceledError('Organization changed')
    }
    if (response.config.acceptAuthRotation && response.data?.success === true) {
      applyAuthRotation(response.data.data)
    }

    if (
      !response.config.skipBusinessError &&
      typeof response.data?.success === 'boolean' &&
      !response.data.success
    ) {
      const messageKey = getServerErrorMessageKey(response.data)
      toast.error(
        messageKey
          ? t(messageKey)
          : response.data.message || t('Request failed')
      )
    }
    return response
  },
  async (error) => {
    const config = error?.config as ApiRequestConfig | undefined
    if (axios.isCancel(error)) throw error
    if (
      config?.organizationEpoch !== undefined &&
      config.organizationEpoch !== useOrganizationStore.getState().epoch
    ) {
      throw new CanceledError('Organization changed')
    }
    if (
      error?.response?.status === 403 &&
      error?.response?.data?.code === 'ORG_UNAVAILABLE'
    ) {
      useOrganizationStore.getState().select(null)
      toast.error(t('Organization unavailable. Returning to Personal.'))
      throw new CanceledError('Organization unavailable')
    }
    if (
      error?.response?.status === 403 &&
      error?.response?.data?.code === 'ORG_FORBIDDEN'
    ) {
      const current = useOrganizationStore.getState()
      current.select(current.activeOrgID)
    }
    const skipErrorHandler = config?.skipErrorHandler
    const status = error?.response?.status

    if (status === 401) {
      if (config && !config.skipAuthRefresh && !config.authRetry) {
        config.authRetry = true
        const outcome = await refreshAuthentication()
        if (outcome.kind === 'authenticated') {
          const token = useAuthStore.getState().auth.accessToken
          if (token) {
            config.headers = {
              ...config.headers,
              Authorization: `Bearer ${token}`,
            }
          }
          return api.request(config)
        }

        if (outcome.kind === 'anonymous' || outcome.kind === 'out_of_sync') {
          if (!skipErrorHandler) toast.error(t('Session expired!'))
          redirectToSignIn()
        }
      } else if (config?.authRetry) {
        clearAuthentication(false)
        if (!skipErrorHandler) toast.error(t('Session expired!'))
        redirectToSignIn()
      } else if (!skipErrorHandler) {
        toast.error(t('Session expired!'))
      }
    } else if (!skipErrorHandler) {
      const messageKey = getServerErrorMessageKey(error)
      const message = messageKey
        ? t(messageKey)
        : error?.response?.data?.message ||
          error?.message ||
          t('Request failed')
      toast.error(message)
    }
    throw error
  }
)

api.interceptors.request.use(
  (config) => {
    const path = config.url ?? ''
    const scoped =
      /^\/api\/(org\/|pricing$|token(?:\/|$)|user\/|subscription\/|log\/self|data\/(?:flow\/)?self|mj\/self|task\/)/.test(
        path
      ) || path.startsWith('/pg/')
    if (scoped && !config.skipOrganizationContext) {
      const scope = useOrganizationStore.getState()
      if (config.organizationEpoch === undefined) {
        config.organizationEpoch = scope.epoch
        config.organizationID = scope.activeOrgID
      }
      if (config.organizationEpoch !== scope.epoch) {
        throw new CanceledError('Organization changed')
      }
      if (config.organizationID) {
        config.headers['X-Org-Id'] = String(config.organizationID)
      }
    }
    const accessToken = useAuthStore.getState().auth.accessToken
    if (accessToken) {
      config.headers.Authorization = `Bearer ${accessToken}`
    }
    return config
  },
  undefined,
  { synchronous: true }
)
