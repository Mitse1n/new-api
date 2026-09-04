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
import { useAuthStore } from '@/stores/auth-store'
import { useOrganizationStore } from '@/stores/organization-store'

import { listOrganizations, changeOrganizationStatus } from '../api'
import { useOrganization, useSwitchOrganization } from '../context'

import '@/styles/multi-tenancy.css'

export function OrganizationSwitcher() {
  const { t } = useTranslation()
  const context = useOrganization()
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
          {context.logo && context.organization.kind === 'team' ? (
            <img
              src={context.logo}
              alt=''
              className='size-5 rounded object-contain'
            />
          ) : (
            <HugeiconsIcon
              icon={
                context.organization.kind === 'personal'
                  ? UserIcon
                  : Building03Icon
              }
              size={18}
            />
          )}
        </span>
        <span className='mt-org-name'>
          {context.organization.kind === 'personal'
            ? t('Personal')
            : context.organization.name}
        </span>
        {context.organization.kind === 'team' && (
          <Badge variant='outline'>{roleLabels[context.membership.role]}</Badge>
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
        {(['personal', 'team'] as const).map((kind) => (
          <div key={kind}>
            <p className='mt-menu-label'>
              {t(kind === 'personal' ? 'Personal' : 'Organization')}
            </p>
            {organizations
              .filter(
                (o) =>
                  o.kind === kind &&
                  (o.kind === 'personal' ? t('Personal') : o.name)
                    .toLowerCase()
                    .includes(search.toLowerCase())
              )
              .map((org) => {
                const name = org.kind === 'personal' ? t('Personal') : org.name
                return (
                  <button
                    type='button'
                    className='mt-org-option'
                    aria-label={
                      org.status === 1
                        ? name
                        : t('Restore {{name}}', {
                            name,
                          })
                    }
                    disabled={restore.isPending}
                    key={org.id}
                    onClick={() => {
                      if (org.status !== 1) {
                        restore.mutate(org.id)
                        return
                      }
                      switchOrg(org.id)
                      setOpen(false)
                      setSearch('')
                      toast.success(
                        t('Switched to {{name}}', {
                          name,
                        })
                      )
                    }}
                  >
                    <span className='mt-org-icon blue'>
                      <HugeiconsIcon
                        icon={
                          org.kind === 'personal' ? UserIcon : Building03Icon
                        }
                        size={18}
                      />
                    </span>
                    <span>
                      <strong>{name}</strong>
                      {org.kind === 'team' && (
                        <small>
                          {org.slug} ·{' '}
                          {org.status === 1
                            ? roleLabels[org.role]
                            : t('Disabled — click to restore')}
                        </small>
                      )}
                    </span>
                    {org.id === context.organization.id && (
                      <HugeiconsIcon icon={Tick02Icon} size={16} />
                    )}
                  </button>
                )
              })}
          </div>
        ))}
      </PopoverContent>
    </Popover>
  )
}
