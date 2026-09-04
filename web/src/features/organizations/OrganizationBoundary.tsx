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
import { useQuery } from '@tanstack/react-query'
import { useEffect, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { useAuthStore } from '@/stores/auth-store'
import { useOrganizationStore } from '@/stores/organization-store'

import { getOrganizationContext, listOrganizations } from './api'

export function OrganizationBoundary(props: { children: ReactNode }) {
  const { t } = useTranslation()
  const userID = useAuthStore((state) => state.auth.user?.id)
  const boundUserID = useOrganizationStore((state) => state.userID)
  const activeOrgID = useOrganizationStore((state) => state.activeOrgID)
  const epoch = useOrganizationStore((state) => state.epoch)
  const context = useOrganizationStore((state) => state.context)
  useEffect(() => {
    if (userID) useOrganizationStore.getState().bindUser(userID)
  }, [userID])
  const organizations = useQuery({
    queryKey: ['organizations', userID, epoch],
    queryFn: listOrganizations,
    enabled: !!userID && userID === boundUserID,
    staleTime: 0,
  })
  useEffect(() => {
    if (!organizations.data) return
    const selected = organizations.data.find(
      (org) => org.id === activeOrgID && org.status === 1
    )
    if (!selected) {
      const personal = organizations.data.find(
        (org) => org.kind === 'personal' && org.status === 1
      )
      if (personal && personal.id !== activeOrgID) {
        useOrganizationStore.getState().select(personal.id)
      }
    }
  }, [organizations.data, activeOrgID])
  const selection = useQuery({
    queryKey: ['organization-context', userID, activeOrgID, epoch],
    queryFn: getOrganizationContext,
    enabled: !!activeOrgID && boundUserID === userID,
    staleTime: 0,
    retry: false,
  })
  useEffect(() => {
    if (selection.data) {
      useOrganizationStore.getState().setContext(selection.data, epoch)
    }
  }, [selection.data, epoch])
  if (
    userID !== boundUserID ||
    !context ||
    context.organization.id !== activeOrgID
  ) {
    const failed =
      organizations.isError ||
      selection.isError ||
      (organizations.isSuccess &&
        !organizations.data.some(
          (org) => org.kind === 'personal' && org.status === 1
        ))
    return (
      <div
        className='flex min-h-svh flex-col items-center justify-center gap-4'
        role='status'
      >
        {failed ? (
          <>
            <p>{t('Unable to load organization')}</p>
            <Button
              onClick={() => {
                void organizations.refetch()
                void selection.refetch()
              }}
            >
              {t('Retry')}
            </Button>
          </>
        ) : (
          <>
            <Spinner />
            <p>{t('Loading organization')}</p>
          </>
        )}
      </div>
    )
  }
  return (
    <div key={`${userID}:${epoch}`} className='contents'>
      {props.children}
    </div>
  )
}
