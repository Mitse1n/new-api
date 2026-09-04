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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { copyToClipboard } from '@/lib/copy-to-clipboard'
import { getCurrencyDisplay } from '@/lib/currency'

import { organizationMutation } from '../api'
import { useOrganization } from '../context'
import type { OrganizationInvite, OrganizationMember } from '../types'

export function MemberDialog(props: {
  member?: OrganizationMember
  budget?: boolean
  close: () => void
}) {
  const { t } = useTranslation()
  const context = useOrganization()
  const client = useQueryClient()
  const [inviteURL, setInviteURL] = useState('')
  const schema = z.object({
    email: z.string().email(t('Enter a valid email address')).or(z.literal('')),
    role: z.enum(['admin', 'member']),
    status: z.enum(['1', '2', '3']),
    limit: z
      .number()
      .min(0)
      .max(Number.MAX_SAFE_INTEGER / getCurrencyDisplay().config.quotaPerUnit),
  })
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: {
      email: props.member?.email ?? '',
      role: props.member?.role === 'admin' ? 'admin' : 'member',
      status: String(props.member?.status ?? 1) as '1' | '2' | '3',
      limit:
        (props.member?.spend_limit ?? 0) /
        getCurrencyDisplay().config.quotaPerUnit,
    },
  })
  const mutation = useMutation({
    mutationFn: async (values: z.infer<typeof schema>) => {
      if (props.member) {
        const spend_limit = Math.round(
          values.limit * getCurrencyDisplay().config.quotaPerUnit
        )
        const path = `members/${props.member.user_id}${props.budget ? '/budget' : ''}`
        await organizationMutation('put', path, {
          role: values.role,
          status: Number(values.status),
          spend_limit,
        })
      } else {
        if (!values.email) {
          form.setError('email', { message: t('Enter a valid email address') })
          return
        }
        const result = await organizationMutation<{
          invite: OrganizationInvite
          token: string
        }>('post', 'invites', { email: values.email, role: values.role })
        const url = new URL('/organization/invite', window.location.origin)
        url.searchParams.set('token', result.token)
        setInviteURL(url.toString())
      }
      await client.invalidateQueries({ queryKey: ['organization-members'] })
      await client.invalidateQueries({ queryKey: ['organization-invites'] })
      await client.invalidateQueries({ queryKey: ['organization-summary'] })
      if (props.member) props.close()
    },
  })
  let title = t('Invite member')
  if (props.member) {
    title = props.budget ? t('Set spending limit') : t('Edit member')
  }
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !mutation.isPending) props.close()
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>
            {context.organization.name} ·{' '}
            {t('Changes apply to this organization only.')}
          </DialogDescription>
        </DialogHeader>
        {inviteURL ? (
          <FieldGroup>
            <Field>
              <FieldLabel>{t('Invitation link')}</FieldLabel>
              <Input
                aria-label={t('Invitation link')}
                readOnly
                value={inviteURL}
              />
              <FieldDescription>
                {t(
                  'Copy this link now. It expires in seven days and can only be accepted by the invited email address.'
                )}
              </FieldDescription>
            </Field>
            <Button
              onClick={() => {
                void copyToClipboard(inviteURL)
              }}
            >
              {t('Copy invitation link')}
            </Button>
            <Button variant='outline' onClick={props.close}>
              {t('Done')}
            </Button>
          </FieldGroup>
        ) : (
          <form
            onSubmit={form.handleSubmit((values) => mutation.mutate(values))}
          >
            <FieldGroup>
              {!props.budget && (
                <>
                  <Field data-invalid={!!form.formState.errors.email}>
                    <FieldLabel htmlFor='member-email'>{t('Email')}</FieldLabel>
                    <Input
                      id='member-email'
                      type='email'
                      disabled={!!props.member}
                      aria-invalid={!!form.formState.errors.email}
                      {...form.register('email')}
                    />
                    <FieldDescription>
                      {form.formState.errors.email?.message}
                    </FieldDescription>
                  </Field>
                  <Field>
                    <FieldLabel htmlFor='member-role'>{t('Role')}</FieldLabel>
                    <NativeSelect id='member-role' {...form.register('role')}>
                      <NativeSelectOption value='member'>
                        {t('Member')}
                      </NativeSelectOption>
                      <NativeSelectOption value='admin'>
                        {t('Admin')}
                      </NativeSelectOption>
                    </NativeSelect>
                  </Field>
                </>
              )}
              {props.member && (
                <>
                  <Field data-invalid={!!form.formState.errors.limit}>
                    <FieldLabel htmlFor='member-limit'>
                      {t('Spending limit (USD)')}
                    </FieldLabel>
                    <Input
                      id='member-limit'
                      type='number'
                      step='0.01'
                      min='0'
                      {...form.register('limit', { valueAsNumber: true })}
                    />
                    <FieldDescription>
                      {t(
                        'Zero means unlimited. This limit does not reserve money from the shared pool.'
                      )}
                    </FieldDescription>
                  </Field>
                  {!props.budget && (
                    <Field>
                      <FieldLabel htmlFor='member-status'>
                        {t('Status')}
                      </FieldLabel>
                      <NativeSelect
                        id='member-status'
                        {...form.register('status')}
                      >
                        <NativeSelectOption value='1'>
                          {t('Active')}
                        </NativeSelectOption>
                        <NativeSelectOption value='2'>
                          {t('Disabled')}
                        </NativeSelectOption>
                        <NativeSelectOption value='3'>
                          {t('Removed')}
                        </NativeSelectOption>
                      </NativeSelect>
                      <FieldDescription>
                        {t(
                          'Existing API keys remain organization assets when a member leaves.'
                        )}
                      </FieldDescription>
                    </Field>
                  )}
                </>
              )}
              {mutation.isError && (
                <p role='alert' className='text-destructive text-sm'>
                  {mutation.error.message}
                </p>
              )}
              <div className='flex justify-end gap-2'>
                <Button variant='outline' type='button' onClick={props.close}>
                  {t('Cancel')}
                </Button>
                <Button type='submit' disabled={mutation.isPending}>
                  {t('Confirm')}
                </Button>
              </div>
            </FieldGroup>
          </form>
        )}
      </DialogContent>
    </Dialog>
  )
}
