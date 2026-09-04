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
import { useMutation } from '@tanstack/react-query'
import type { Row } from '@tanstack/react-table'
import { Edit, Power, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

import { updateApiKeyStatus } from '../api'
import { API_KEY_STATUS } from '../constants'
import { apiKeySchema } from '../types'
import { useApiKeys } from './api-keys-context'

export function DataTableRowActions<TData>(props: { row: Row<TData> }) {
  const { t } = useTranslation()
  const apiKey = apiKeySchema.parse(props.row.original)
  const { setOpen, setCurrentRow, triggerRefresh } = useApiKeys()
  const enabled = apiKey.status === API_KEY_STATUS.ENABLED
  const toggle = useMutation({
    mutationFn: () =>
      updateApiKeyStatus(
        apiKey.id,
        enabled ? API_KEY_STATUS.DISABLED : API_KEY_STATUS.ENABLED
      ),
    onSuccess: triggerRefresh,
  })
  return (
    <div className='flex items-center gap-1'>
      <Button
        variant='ghost'
        size='icon-sm'
        aria-label={enabled ? t('Disable') : t('Enable')}
        disabled={toggle.isPending}
        onClick={() => toggle.mutate()}
      >
        <Power />
      </Button>
      <Button
        variant='ghost'
        size='icon-sm'
        aria-label={t('Edit')}
        onClick={() => {
          setCurrentRow(apiKey)
          setOpen('update')
        }}
      >
        <Edit />
      </Button>
      <Button
        variant='ghost'
        size='icon-sm'
        aria-label={t('Delete')}
        onClick={() => {
          setCurrentRow(apiKey)
          setOpen('delete')
        }}
      >
        <Trash2 />
      </Button>
    </div>
  )
}
