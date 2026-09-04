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
import { useMutation } from '@tanstack/react-query'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'

import { createOrganization } from '../api'
import { useSwitchOrganization } from '../context'

import '@/styles/multi-tenancy.css'

export function CreateOrganization() {
  const { t } = useTranslation()
  const switchOrg = useSwitchOrganization()
  const creation = useMutation({ mutationFn: createOrganization })
  const [creating, setCreating] = useState(false)
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
      <Button
        onClick={() => {
          reset({ name: '', slug: '' })
          setCreating(true)
        }}
      >
        {t('Create organization')}
      </Button>
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
