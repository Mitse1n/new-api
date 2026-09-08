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
  ArrowDown01Icon,
  Tick02Icon,
  Building03Icon,
  UserIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { Separator } from '@/components/ui/separator'
import { useAuthStore } from '@/stores/auth-store'
import { useOrganizationStore } from '@/stores/organization-store'

import { listOrganizations, changeOrganizationStatus } from '../api'
import { useSwitchOrganization, useTeamContext } from '../context'

import '@/styles/multi-tenancy.css'

export function OrganizationSwitcher() {
  const { t } = useTranslation()
  const team = useTeamContext()
  const switchOrg = useSwitchOrganization()
  const userID = useAuthStore((state) => state.auth.user?.id)
  const epoch = useOrganizationStore((state) => state.epoch)
  const list = useQuery({
    queryKey: ['organizations', userID, epoch],
    queryFn: listOrganizations,
  })
  const organizations = list.data ?? []
  const roleLabels = {
    owner: t('Owner'),
    admin: t('Admin'),
    member: t('Member'),
  }
  const restore = useMutation({
    mutationFn: (id: number) => changeOrganizationStatus(id, 1),
    onSuccess: () => {
      void list.refetch()
    },
  })
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={
          <Button
            variant='ghost'
            className='mt-org-trigger'
            aria-label={t('Switch organization')}
          />
        }
      >
        <span className='mt-org-icon blue'>
          {team?.logo ? (
            <img
              src={team.logo}
              alt=''
              className='size-5 rounded object-contain'
            />
          ) : (
            <HugeiconsIcon
              icon={team === null ? UserIcon : Building03Icon}
              size={18}
            />
          )}
        </span>
        <span className='mt-org-name'>
          {team === null ? t('Personal') : team.organization.name}
        </span>
        {team && (
          <Badge variant='outline'>{roleLabels[team.membership.role]}</Badge>
        )}
        <HugeiconsIcon icon={ArrowDown01Icon} size={16} />
      </PopoverTrigger>
      <PopoverContent align='start' className='mt-org-popover'>
        <Input
          autoFocus
          placeholder={t('Search organizations')}
          aria-label={t('Search organizations')}
          value={search}
          onChange={(event) => setSearch(event.target.value)}
        />
        {t('Personal').toLowerCase().includes(search.toLowerCase()) && (
          <button
            type='button'
            className='mt-org-option'
            aria-label={t('Personal')}
            onClick={() => {
              switchOrg(null)
              setOpen(false)
              setSearch('')
              toast.success(t('Switched to {{name}}', { name: t('Personal') }))
            }}
          >
            <span className='mt-org-icon blue'>
              <HugeiconsIcon icon={UserIcon} size={18} />
            </span>
            <strong>{t('Personal')}</strong>
            {team === null && <HugeiconsIcon icon={Tick02Icon} size={16} />}
          </button>
        )}
        <Separator className='mt-2' />
        <p className='mt-menu-label'>{t('Organization')}</p>
        {organizations
          .filter((org) =>
            org.name.toLowerCase().includes(search.toLowerCase())
          )
          .map((org) => (
            <button
              type='button'
              className='mt-org-option'
              key={org.id}
              aria-label={
                org.status === 1
                  ? org.name
                  : t('Restore {{name}}', { name: org.name })
              }
              disabled={restore.isPending}
              onClick={() => {
                if (org.status !== 1) {
                  restore.mutate(org.id)
                  return
                }
                switchOrg(org.id)
                setOpen(false)
                setSearch('')
                toast.success(t('Switched to {{name}}', { name: org.name }))
              }}
            >
              <span className='mt-org-icon blue'>
                <HugeiconsIcon icon={Building03Icon} size={18} />
              </span>
              <span>
                <strong>{org.name}</strong>
                <small>
                  {org.slug} ·{' '}
                  {org.status === 1
                    ? roleLabels[org.role]
                    : t('Disabled — click to restore')}
                </small>
              </span>
              {org.id === team?.organization.id && (
                <HugeiconsIcon icon={Tick02Icon} size={16} />
              )}
            </button>
          ))}
      </PopoverContent>
    </Popover>
  )
}
