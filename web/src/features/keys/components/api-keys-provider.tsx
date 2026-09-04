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
import React, { useCallback, useState } from 'react'

import useDialogState from '@/hooks/use-dialog'

import type { ApiKey, ApiKeysDialogType } from '../types'
import { ApiKeysContext } from './api-keys-context'

export function ApiKeysProvider(props: { children: React.ReactNode }) {
  const [open, setOpen] = useDialogState<ApiKeysDialogType>(null)
  const [currentRow, setCurrentRow] = useState<ApiKey | null>(null)
  const [refreshTrigger, setRefreshTrigger] = useState(0)
  const [createdSecrets, setCreatedSecrets] = useState<
    Array<{ name: string; key: string }>
  >([])
  const triggerRefresh = useCallback(
    () => setRefreshTrigger((value) => value + 1),
    []
  )
  return (
    <ApiKeysContext
      value={{
        open,
        setOpen,
        currentRow,
        setCurrentRow,
        refreshTrigger,
        triggerRefresh,
        createdSecrets,
        setCreatedSecrets,
      }}
    >
      {props.children}
    </ApiKeysContext>
  )
}
