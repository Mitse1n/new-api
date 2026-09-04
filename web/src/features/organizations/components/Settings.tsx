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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
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
import { Textarea } from '@/components/ui/textarea'
import { getCurrencyDisplay } from '@/lib/currency'

import {
  changeOrganizationStatus,
  getDeletionImpact,
  getOrganizationMembers,
  getOrganizationSettings,
  organizationMutation,
  updateOrganizationSettings,
} from '../api'
import { useOrganization, useSwitchOrganization } from '../context'
import type { OrganizationSettingsResponse } from '../types'

export function Settings() {
  const { t } = useTranslation()
  const context = useOrganization()
  const settings = useQuery({
    queryKey: ['organization-settings', context.organization.id],
    queryFn: getOrganizationSettings,
  })
  if (!settings.data) {
    return (
      <p role='status'>
        {settings.isError ? t('Unable to load organization') : t('Loading...')}
      </p>
    )
  }
  return <SettingsForm initial={settings.data} />
}

function SettingsForm(props: { initial: OrganizationSettingsResponse }) {
  const { t } = useTranslation()
  const context = useOrganization()
  const client = useQueryClient()
  const switchOrg = useSwitchOrganization()
  const [action, setAction] = useState<
    'disable' | 'delete' | 'transfer' | null
  >(null)
  const [confirmSlug, setConfirmSlug] = useState('')
  const [target, setTarget] = useState('')
  const schema = z.object({
    name: z.string().trim().min(1).max(64),
    logo: z.string(),
    webhook: z.string(),
    alert_email: z.string().email().or(z.literal('')),
    default_limit: z.number().nonnegative(),
    budget: z.number().nonnegative(),
    percent: z.number().int().min(1).max(100),
    models: z.string(),
  })
  const unit = getCurrencyDisplay().config.quotaPerUnit
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: {
      name: props.initial.name,
      logo: props.initial.settings.logo,
      webhook: props.initial.settings.webhook,
      alert_email: props.initial.settings.alert_email,
      default_limit: props.initial.settings.default_spend_limit / unit,
      budget: props.initial.settings.budget_limit / unit,
      percent: props.initial.settings.alert_percent,
      models: props.initial.settings.allowed_models.join('\n'),
    },
  })
  const update = useMutation({
    mutationFn: async (values: z.infer<typeof schema>) => {
      await updateOrganizationSettings({
        name: values.name,
        settings: {
          logo: values.logo,
          webhook: values.webhook,
          alert_email: values.alert_email,
          default_spend_limit: Math.round(values.default_limit * unit),
          budget_limit: Math.round(values.budget * unit),
          alert_percent: values.percent,
          allowed_models: values.models
            .split('\n')
            .map((model) => model.trim())
            .filter(Boolean),
        },
      })
      await client.invalidateQueries({ queryKey: ['organization-context'] })
      await client.invalidateQueries({ queryKey: ['organization-settings'] })
    },
  })
  const members = useQuery({
    queryKey: ['organization-members', context.organization.id],
    queryFn: getOrganizationMembers,
    enabled: action === 'transfer',
  })
  const impact = useQuery({
    queryKey: ['organization-deletion-impact', context.organization.id],
    queryFn: () => getDeletionImpact(context.organization.id),
    enabled: action === 'delete',
    staleTime: 0,
  })
  const lifecycle = useMutation({
    mutationFn: async () => {
      if (action === 'transfer') {
        await organizationMutation('post', 'transfer', {
          target_id: Number(target),
        })
        setAction(null)
        return
      }
      await changeOrganizationStatus(
        context.organization.id,
        action === 'delete' ? 3 : 2,
        confirmSlug
      )
      switchOrg(context.organization.id)
    },
  })
  let actionTitle = t('Disable organization')
  if (action === 'delete') actionTitle = t('Delete organization')
  if (action === 'transfer') actionTitle = t('Transfer ownership')
  const owner =
    context.membership.role === 'owner' && context.organization.kind === 'team'
  return (
    <div className='flex flex-col gap-5'>
      <Card>
        <CardHeader>
          <CardTitle>{t('Organization settings')}</CardTitle>
        </CardHeader>
        <CardContent>
          <form onSubmit={form.handleSubmit((values) => update.mutate(values))}>
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor='org-name'>
                  {t('Organization name')}
                </FieldLabel>
                <Input
                  id='org-name'
                  maxLength={64}
                  {...form.register('name')}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor='org-logo'>{t('Logo URL')}</FieldLabel>
                <Input id='org-logo' type='url' {...form.register('logo')} />
              </Field>
              <div className='grid gap-4 md:grid-cols-2'>
                <Field>
                  <FieldLabel htmlFor='org-email'>
                    {t('Alert email')}
                  </FieldLabel>
                  <Input
                    id='org-email'
                    type='email'
                    {...form.register('alert_email')}
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor='org-webhook'>
                    {t('Notification webhook')}
                  </FieldLabel>
                  <Input
                    id='org-webhook'
                    type='url'
                    {...form.register('webhook')}
                  />
                </Field>
              </div>
              <div className='grid gap-4 md:grid-cols-3'>
                <Field>
                  <FieldLabel htmlFor='org-default-limit'>
                    {t('Default member limit (USD)')}
                  </FieldLabel>
                  <Input
                    id='org-default-limit'
                    type='number'
                    min='0'
                    step='0.01'
                    {...form.register('default_limit', { valueAsNumber: true })}
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor='org-budget'>
                    {t('Organization budget (USD)')}
                  </FieldLabel>
                  <Input
                    id='org-budget'
                    type='number'
                    min='0'
                    step='0.01'
                    {...form.register('budget', { valueAsNumber: true })}
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor='org-alert'>
                    {t('Alert threshold (%)')}
                  </FieldLabel>
                  <Input
                    id='org-alert'
                    type='number'
                    min='1'
                    max='100'
                    {...form.register('percent', { valueAsNumber: true })}
                  />
                </Field>
              </div>
              <Field>
                <FieldLabel htmlFor='org-models'>
                  {t('Allowed models')}
                </FieldLabel>
                <Textarea
                  id='org-models'
                  rows={5}
                  {...form.register('models')}
                />
                <FieldDescription>
                  {t(
                    'One model per line. Leave empty to use all models included in your plan. This cannot grant access to additional models.'
                  )}
                </FieldDescription>
                <details>
                  <summary>{t('Models included in your plan')}</summary>
                  <p className='mt-2 text-sm break-words'>
                    {props.initial.available_models.join(', ')}
                  </p>
                </details>
              </Field>
              {Object.keys(form.formState.errors).length > 0 && (
                <p role='alert' className='text-destructive'>
                  {t('Please fix the highlighted fields before saving')}
                </p>
              )}
              {update.isError && (
                <p role='alert' className='text-destructive'>
                  {update.error.message}
                </p>
              )}
              {update.isSuccess && <p role='status'>{t('Changes saved')}</p>}
              <Button
                type='submit'
                className='self-end'
                disabled={update.isPending}
              >
                {t('Save changes')}
              </Button>
            </FieldGroup>
          </form>
        </CardContent>
      </Card>
      {owner && (
        <Card>
          <CardHeader>
            <CardTitle>{t('Danger zone')}</CardTitle>
          </CardHeader>
          <CardContent className='flex flex-wrap gap-3'>
            <Button variant='outline' onClick={() => setAction('transfer')}>
              {t('Transfer ownership')}
            </Button>
            <Button variant='outline' onClick={() => setAction('disable')}>
              {t('Disable organization')}
            </Button>
            <Button variant='destructive' onClick={() => setAction('delete')}>
              {t('Delete organization')}
            </Button>
          </CardContent>
        </Card>
      )}
      <Dialog
        open={action !== null}
        onOpenChange={(open) => {
          if (!open && !lifecycle.isPending) setAction(null)
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{actionTitle}</DialogTitle>
            <DialogDescription>{context.organization.name}</DialogDescription>
          </DialogHeader>
          {action === 'transfer' && (
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor='transfer-target'>
                  {t('New owner')}
                </FieldLabel>
                <NativeSelect
                  id='transfer-target'
                  value={target}
                  onChange={(event) => setTarget(event.target.value)}
                >
                  <NativeSelectOption value=''>
                    {t('Select a member')}
                  </NativeSelectOption>
                  {members.data
                    ?.filter(
                      (member) => member.status === 1 && member.role !== 'owner'
                    )
                    .map((member) => (
                      <NativeSelectOption
                        key={member.user_id}
                        value={member.user_id}
                      >
                        {member.display_name || member.username}
                      </NativeSelectOption>
                    ))}
                </NativeSelect>
                <FieldDescription>
                  {t(
                    'Ownership changes only after this member accepts. You will become an Admin.'
                  )}
                </FieldDescription>
              </Field>
            </FieldGroup>
          )}
          {action === 'disable' && (
            <p>
              {t(
                'Organization access and API keys will stop working. You can restore the team from the organization switcher.'
              )}
            </p>
          )}
          {action === 'delete' && (
            <FieldGroup>
              <p>
                {t(
                  'Deletion is blocked while a balance, active subscription, pending payment or unfinished request remains. Disable the team and contact the platform to settle funds.'
                )}
              </p>
              {impact.data && (
                <p>
                  {t(
                    'Affected: {{members}} members, {{keys}} keys, {{logs}} logs, {{orders}} orders and {{subscriptions}} subscriptions.',
                    { ...impact.data, keys: impact.data.tokens }
                  )}
                </p>
              )}
              <Field>
                <FieldLabel htmlFor='confirm-slug'>
                  {t('Type {{slug}} to confirm deletion', {
                    slug: context.organization.slug,
                  })}
                </FieldLabel>
                <Input
                  id='confirm-slug'
                  value={confirmSlug}
                  onChange={(event) => setConfirmSlug(event.target.value)}
                />
              </Field>
            </FieldGroup>
          )}
          {lifecycle.isError && (
            <p role='alert' className='text-destructive'>
              {lifecycle.error.message}
            </p>
          )}
          <div className='flex justify-end gap-2'>
            <Button variant='outline' onClick={() => setAction(null)}>
              {t('Cancel')}
            </Button>
            <Button
              variant='destructive'
              disabled={
                lifecycle.isPending ||
                (action === 'delete' &&
                  (!impact.data ||
                    impact.data.blocked ||
                    confirmSlug !== context.organization.slug)) ||
                (action === 'transfer' && !target)
              }
              onClick={() => lifecycle.mutate()}
            >
              {t('Confirm')}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  )
}
