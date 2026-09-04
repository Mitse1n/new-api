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
import { act, cleanup, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { getFreshAuthHeaders } from '@/lib/api'
import { useOrganizationStore } from '@/stores/organization-store'

import type { ChatCompletionRequest } from '../../types'
import { useStreamRequest } from '../use-stream-request'

const sources = vi.hoisted(() => ({
  options: [] as Array<{ headers: Record<string, string> }>,
  streams: [] as Array<{
    close: ReturnType<typeof vi.fn>
    stream: ReturnType<typeof vi.fn>
    listeners: Map<string, (event: Event & { data?: string }) => void>
  }>,
}))
vi.mock('@/lib/api', () => ({ getFreshAuthHeaders: vi.fn() }))
vi.mock('sse.js', () => ({
  SSE: class {
    constructor(_url: string, options: { headers: Record<string, string> }) {
      sources.options.push(options)
      sources.streams.push(this)
    }
    listeners = new Map<string, (event: Event & { data?: string }) => void>()
    close = vi.fn()
    stream = vi.fn()
    addEventListener(
      type: string,
      listener: (event: Event & { data?: string }) => void
    ) {
      this.listeners.set(type, listener)
    }
  },
}))

const payload: ChatCompletionRequest = {
  model: 'test-model',
  messages: [{ role: 'user', content: 'test' }],
  stream: true,
}
const onUpdate = vi.fn()
const onComplete = vi.fn()
const onError = vi.fn()

beforeEach(() => {
  vi.clearAllMocks()
  sources.options.length = 0
  sources.streams.length = 0
  localStorage.clear()
  useOrganizationStore.setState(useOrganizationStore.getInitialState(), true)
  useOrganizationStore.getState().bindUser(7)
  useOrganizationStore.getState().select(22)
  vi.mocked(getFreshAuthHeaders).mockResolvedValue({
    Authorization: 'Bearer fixture',
  })
})
afterEach(() => {
  cleanup()
  useOrganizationStore.setState(useOrganizationStore.getInitialState(), true)
  localStorage.clear()
})

test.each([1, 22])(
  'streaming sends the explicitly selected personal or team organization %s',
  async (orgID) => {
    useOrganizationStore.getState().select(orgID)
    const { result } = renderHook(useStreamRequest)
    await act(async () => {
      await result.current.sendStreamRequest(
        payload,
        onUpdate,
        onComplete,
        onError
      )
    })
    expect(sources.options[0]?.headers).toEqual({
      Authorization: 'Bearer fixture',
      'X-Org-Id': String(orgID),
    })
    expect(sources.streams[0]?.stream).toHaveBeenCalledOnce()
  }
)

test('switching organizations during authentication cancels the old request even after switching back', async () => {
  let resolve!: (headers: Record<string, string>) => void
  vi.mocked(getFreshAuthHeaders).mockReturnValueOnce(
    new Promise((done) => {
      resolve = done
    })
  )
  const { result } = renderHook(useStreamRequest)
  let pending: Promise<void> | undefined
  act(() => {
    pending = result.current.sendStreamRequest(
      payload,
      onUpdate,
      onComplete,
      onError
    )
  })
  await act(async () => {
    useOrganizationStore.getState().select(33)
    useOrganizationStore.getState().select(22)
    resolve({ Authorization: 'Bearer refreshed' })
    await pending
  })
  expect(sources.streams).toHaveLength(0)
  expect(onError).not.toHaveBeenCalled()
  await act(async () => {
    await result.current.sendStreamRequest(
      payload,
      onUpdate,
      onComplete,
      onError
    )
  })
  expect(sources.options[0]?.headers['X-Org-Id']).toBe('22')
})

test('switching organizations closes an active stream and ignores its late output', async () => {
  const { result } = renderHook(useStreamRequest)
  await act(async () => {
    await result.current.sendStreamRequest(
      payload,
      onUpdate,
      onComplete,
      onError
    )
  })
  act(() => {
    useOrganizationStore.getState().select(33)
    sources.streams[0]?.listeners.get('message')?.({
      data: JSON.stringify({
        choices: [{ delta: { content: 'old organization' } }],
      }),
    } as Event & { data: string })
  })
  expect(sources.streams[0]?.close).toHaveBeenCalledOnce()
  expect(result.current.isStreaming).toBe(false)
  expect(onUpdate).not.toHaveBeenCalled()
  await act(async () => {
    await result.current.sendStreamRequest(
      payload,
      onUpdate,
      onComplete,
      onError
    )
  })
  expect(sources.options[1]?.headers['X-Org-Id']).toBe('33')
})
