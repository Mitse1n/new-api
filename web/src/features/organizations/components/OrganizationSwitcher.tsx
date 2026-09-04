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
import { zodResolver } from '@hookform/resolvers/zod'
import {
  ArrowDown01Icon,
  Add01Icon,
  Tick02Icon,
  Building03Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { useAuthStore } from '@/stores/auth-store'
import { useOrganizationStore } from '@/stores/organization-store'

import {
  createOrganization,
  listOrganizations,
  changeOrganizationStatus,
} from '../api'
import { useOrganization, useSwitchOrganization } from '../context'

import '@/styles/multi-tenancy.css'

export function OrganizationSwitcher() {
  const { t } = useTranslation()
  const context = useOrganization()
  const switchOrg = useSwitchOrganization()
  const userID = useAuthStore((state) => state.auth.user?.id)
  const epoch = useOrganizationStore((state) => state.epoch)
  const platform = useOrganizationStore((state) => state.platform)
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
  const canEnterPlatform = Object.values(context.capabilities.platform).some(
    (actions) => Object.values(actions).some(Boolean)
  )
  const creation = useMutation({ mutationFn: createOrganization })
  const restore = useMutation({
    mutationFn: (id: number) => changeOrganizationStatus(id, 1),
    onSuccess: () => {
      void list.refetch()
    },
  })
  const [open, setOpen] = useState(false)
  const [creating, setCreating] = useState(false)
  const [search, setSearch] = useState('')
  const {
    register,
    handleSubmit,
    setValue,
    getValues,
    reset,
    setError,
    formState: { errors, dirtyFields },
  } = useForm<{ name: string; slug: string }>({
    resolver: zodResolver(
      z.object({
        name: z
          .string()
          .trim()
          .min(1, t('Organization name is required'))
          .max(64),
        slug: z
          .string()
          .min(1)
          .max(64)
          .regex(
            /^[a-z0-9]+(?:-[a-z0-9]+)*$/,
            t('Use lowercase letters, numbers and hyphens.')
          ),
      })
    ),
  })
  return (
    <>
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
            {context.logo && !platform ? (
              <img
                src={context.logo}
                alt=''
                className='size-5 rounded object-contain'
              />
            ) : (
              <HugeiconsIcon icon={Building03Icon} size={18} />
            )}
          </span>
          <span className='mt-org-name'>
            {platform
              ? t('Platform administration')
              : context.organization.name}
          </span>
          <Badge variant='outline'>{roleLabels[context.membership.role]}</Badge>
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
                {t(
                  kind === 'personal'
                    ? 'Personal organizations'
                    : 'Team organizations'
                )}
              </p>
              {organizations
                .filter(
                  (o) =>
                    o.kind === kind &&
                    o.name.toLowerCase().includes(search.toLowerCase())
                )
                .map((org) => (
                  <button
                    type='button'
                    className='mt-org-option'
                    aria-label={
                      org.status === 1
                        ? org.name
                        : t('Restore {{name}}', { name: org.name })
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
                        t('Switched to {{name}}', { name: org.name })
                      )
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
                    {!platform && org.id === context.organization.id && (
                      <HugeiconsIcon icon={Tick02Icon} size={16} />
                    )}
                  </button>
                ))}
            </div>
          ))}
          {canEnterPlatform && (
            <Button
              variant='ghost'
              onClick={() => {
                switchOrg(context.organization.id, !platform)
                setOpen(false)
              }}
            >
              {platform
                ? t('Return to organization')
                : t('Platform administration')}
            </Button>
          )}
          <Button
            variant='outline'
            onClick={() => {
              setOpen(false)
              reset({ name: '', slug: '' })
              setCreating(true)
            }}
          >
            <HugeiconsIcon icon={Add01Icon} data-icon='inline-start' />
            {t('Create organization')}
          </Button>
        </PopoverContent>
      </Popover>
      <Dialog open={creating} onOpenChange={setCreating}>
        <DialogContent className='mt-dialog' showCloseButton={false}>
          <DialogHeader>
            <DialogTitle>{t('Create organization')}</DialogTitle>
            <DialogDescription>
              {t(
                'Bring your team together with shared billing and independent access.'
              )}
            </DialogDescription>
          </DialogHeader>
          <form
            className='mt-form'
            onSubmit={handleSubmit(async (data) => {
              try {
                const org = await creation.mutateAsync(data)
                setCreating(false)
                switchOrg(org.id)
                toast.success(t('Organization created'))
              } catch (error) {
                setError('root', {
                  message:
                    error instanceof Error
                      ? error.message
                      : t('Request failed'),
                })
              }
            })}
          >
            <label>
              {t('Organization name')}
              <Input
                {...register('name', {
                  required: true,
                  onChange: (event) => {
                    if (dirtyFields.slug) return
                    const name = String(event.target.value)
                    const suggested = name
                      .toLowerCase()
                      .replaceAll(/[^a-z0-9]+/g, '-')
                      .replaceAll(/^-|-$/g, '')
                    setValue(
                      'slug',
                      suggested ||
                        getValues('slug') ||
                        `team-${crypto.randomUUID().slice(0, 8)}`
                    )
                  },
                })}
                maxLength={64}
                required
                placeholder={t('Your team name')}
              />
            </label>
            <label>
              {t('Organization slug')}
              <Input
                {...register('slug')}
                required
                maxLength={64}
                placeholder='my-team'
              />
              <small>
                {t('Lowercase letters, numbers and hyphens. Globally unique.')}
              </small>
            </label>
            {(errors.name || errors.slug) && (
              <p role='alert' className='mt-error'>
                {errors.name?.message ?? errors.slug?.message}
              </p>
            )}
            {errors.root && (
              <p role='alert' className='mt-error'>
                {errors.root.message}
              </p>
            )}
            <div className='mt-form-footer'>
              <Button
                type='button'
                variant='outline'
                onClick={() => setCreating(false)}
              >
                {t('Cancel')}
              </Button>
              <Button type='submit' disabled={creation.isPending}>
                {t('Create organization')}
              </Button>
            </div>
          </form>
        </DialogContent>
      </Dialog>
    </>
  )
}
