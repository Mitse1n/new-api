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
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useOrganizationStore } from '@/stores/organization-store'

import { apiKeySchema } from '../../types'
import { ApiKeyCell } from '../api-keys-cells'
import { ApiKeysProvider } from '../api-keys-provider'

const i18n = createInstance()
await i18n
  .use(initReactI18next)
  .init({ lng: 'en', resources: { en: { translation: {} } } })
const originalAdapter = api.defaults.adapter
const key = apiKeySchema.parse({
  id: 1,
  name: 'Personal key',
  key: 'masked',
  status: 1,
  remain_quota: 1,
  used_quota: 0,
  unlimited_quota: true,
  expired_time: -1,
  created_time: 1,
  accessed_time: 1,
  model_limits_enabled: false,
})
afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  api.defaults.adapter = originalAdapter
  useOrganizationStore.setState(useOrganizationStore.getInitialState())
})
test('personal keys can be revealed and are cleared when switching to an organization', async () => {
  useOrganizationStore.setState({ activeOrgID: null, epoch: 0 })
  api.defaults.adapter = async (config) => {
    expect(config.url).toBe('/api/token/1/key')
    return {
      config,
      status: 200,
      statusText: 'OK',
      headers: {},
      data: { success: true, data: { key: 'personal-secret' } },
    }
  }
  render(
    <I18nextProvider i18n={i18n}>
      <ApiKeysProvider>
        <ApiKeyCell apiKey={key} />
      </ApiKeysProvider>
    </I18nextProvider>
  )
  const writeText = vi.fn().mockResolvedValue(undefined)
  vi.stubGlobal('navigator', { clipboard: { writeText } })
  fireEvent.click(screen.getByRole('button', { name: 'sk-masked' }))
  expect(await screen.findByDisplayValue('sk-personal-secret')).toBeVisible()
  fireEvent.click(screen.getByRole('button', { name: 'Copy API key' }))
  await waitFor(() =>
    expect(writeText).toHaveBeenCalledWith('sk-personal-secret')
  )
  useOrganizationStore.getState().select(10)
  await waitFor(() =>
    expect(
      screen.queryByDisplayValue('sk-personal-secret')
    ).not.toBeInTheDocument()
  )
  expect(
    screen.queryByRole('button', { name: 'sk-masked' })
  ).not.toBeInTheDocument()
})
test('organization keys stay masked and do not request persisted secrets', () => {
  useOrganizationStore.setState({ activeOrgID: 10 })
  render(
    <I18nextProvider i18n={i18n}>
      <ApiKeysProvider>
        <ApiKeyCell apiKey={key} />
      </ApiKeysProvider>
    </I18nextProvider>
  )
  expect(screen.getByText('sk-masked')).toHaveAttribute(
    'title',
    'The full key is shown only once, when created.'
  )
  expect(screen.queryByRole('button')).not.toBeInTheDocument()
})
