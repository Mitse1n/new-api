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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { getServerErrorMessageKey } from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

import { acceptOrganizationInvite, declineOrganizationInvite } from '../api'
import { useSwitchOrganization } from '../context'
import type { IncomingOrganizationInvite } from '../types'

export function InvitationNotifications(props: {
  invitations: IncomingOrganizationInvite[]
  onDone: () => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const userID = useAuthStore((state) => state.auth.user?.id)
  const switchOrg = useSwitchOrganization()
  const respond = useMutation({
    mutationFn: async (response: { id: number; accept: boolean }) => {
      if (response.accept) return acceptOrganizationInvite(response.id)
      await declineOrganizationInvite(response.id)
      return null
    },
    onSuccess: (orgID) => {
      if (useAuthStore.getState().auth.user?.id !== userID) return
      if (orgID !== null) {
        toast.success(t('You joined the organization.'))
        props.onDone()
        switchOrg(orgID)
      } else {
        toast.success(t('Invitation declined.'))
      }
    },
    onSettled: () => {
      void client.invalidateQueries({
        queryKey: ['incoming-organization-invites', userID],
      })
    },
  })
  if (props.invitations.length === 0) return null
  return (
    <section
      aria-label={t('Organization invitations')}
      className='space-y-3 border-b pb-3'
    >
      <h3 className='text-sm font-semibold'>{t('Organization invitations')}</h3>
      <div className='max-h-72 space-y-3 overflow-y-auto'>
        {props.invitations.map((invite) => (
          <div key={invite.id} className='space-y-2 rounded-md border p-3'>
            <p className='font-medium break-words'>
              {invite.organization_name}
            </p>
            <p className='text-muted-foreground text-xs'>
              {t('{{username}} invited you to join as {{role}}.', {
                username: invite.inviter_username,
                role: t(invite.role === 'admin' ? 'Admin' : 'Member'),
              })}
            </p>
            <p className='text-muted-foreground text-xs'>
              {t('Expires')} ·{' '}
              {new Date(invite.expires_at * 1000).toLocaleDateString()}
            </p>
            <div className='flex gap-2'>
              <Button
                size='sm'
                disabled={respond.isPending}
                onClick={() => respond.mutate({ id: invite.id, accept: true })}
              >
                {t('Accept invitation')}
              </Button>
              <Button
                size='sm'
                variant='outline'
                disabled={respond.isPending}
                onClick={() => respond.mutate({ id: invite.id, accept: false })}
              >
                {t('Decline invitation')}
              </Button>
            </div>
          </div>
        ))}
      </div>
      {respond.isError && (
        <p role='alert' className='text-destructive text-sm'>
          {t(
            getServerErrorMessageKey(respond.error) ??
              'Invitation unavailable or identity does not match'
          )}
        </p>
      )}
    </section>
  )
}
