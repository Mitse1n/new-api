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
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Textarea } from '@/components/ui/textarea'
import { copyToClipboard } from '@/lib/copy-to-clipboard'

import { useApiKeys } from './api-keys-context'

export function CreatedKeysDialog() {
  const { t } = useTranslation()
  const { createdSecrets, setCreatedSecrets } = useApiKeys()
  const value = createdSecrets
    .map((secret) => `${secret.name}\t${secret.key}`)
    .join('\n')
  return (
    <Dialog
      open={createdSecrets.length > 0}
      onOpenChange={(open) => {
        if (!open) setCreatedSecrets([])
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('Save your API keys')}</DialogTitle>
          <DialogDescription>
            {t(
              'The full key is shown only once, when created. Copy it now and keep it secure.'
            )}
          </DialogDescription>
        </DialogHeader>
        <Textarea
          aria-label={t('New API keys')}
          readOnly
          value={value}
          rows={Math.min(10, createdSecrets.length + 1)}
        />
        <Button
          onClick={() => {
            void copyToClipboard(value)
          }}
        >
          {t('Copy keys')}
        </Button>
        <Button variant='outline' onClick={() => setCreatedSecrets([])}>
          {t('I have saved my keys')}
        </Button>
      </DialogContent>
    </Dialog>
  )
}
