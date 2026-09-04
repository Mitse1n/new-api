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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { api } from '@/lib/api'
import { formatQuotaWithCurrency } from '@/lib/currency'

import type { Organization, Page } from './types'

const resourceColumns = {
  members: ['user_id', 'role', 'status', 'spend_limit'],
  logs: ['created_at', 'user_id', 'model_name', 'quota', 'request_id'],
  orders: ['trade_no', 'plan_id', 'user_id', 'money', 'status'],
  subscriptions: ['id', 'plan_id', 'amount_total', 'amount_used', 'status'],
  audit: ['created_at', 'actor_id', 'action', 'object_id', 'result', 'reason'],
} as const

type Resource = keyof typeof resourceColumns

export function PlatformOrganizations() {
  const { t } = useTranslation()
  const client = useQueryClient()
  const [keyword, setKeyword] = useState('')
  const [page, setPage] = useState(1)
  const [selected, setSelected] = useState<Organization | null>(null)
  const [resource, setResource] = useState<Resource>('members')
  const [resourcePage, setResourcePage] = useState(1)
  const [confirm, setConfirm] = useState(false)
  const [reason, setReason] = useState('')
  const organizations = useQuery({
    queryKey: ['platform-organizations', keyword, page],
    queryFn: async () => {
      const response = await api.get<{
        success: boolean
        data: Page<Organization>
      }>('/api/platform/organizations', {
        params: { keyword, p: page, size: 20 },
      })
      if (!response.data.success) throw new Error(t('Request failed'))
      return response.data.data
    },
  })
  const resources = useQuery({
    queryKey: [
      'platform-organization-resources',
      selected?.id,
      resource,
      resourcePage,
    ],
    enabled: selected !== null,
    queryFn: async () => {
      const response = await api.get<{
        success: boolean
        data: Page<Record<string, string | number>>
      }>(`/api/platform/organizations/${selected?.id}/resources/${resource}`, {
        params: { p: resourcePage, size: 20 },
      })
      if (!response.data.success) throw new Error(t('Request failed'))
      return response.data.data
    },
  })
  const status = useMutation({
    mutationFn: async () => {
      if (!selected) return
      const response = await api.put(
        `/api/platform/organizations/${selected.id}/status`,
        { status: selected.status === 1 ? 2 : 1, reason }
      )
      if (!response.data.success) throw new Error(t('Request failed'))
      setSelected({ ...selected, status: selected.status === 1 ? 2 : 1 })
    },
    onSuccess: () => {
      setConfirm(false)
      setReason('')
      void client.invalidateQueries({ queryKey: ['platform-organizations'] })
      void client.invalidateQueries({
        queryKey: ['platform-organization-resources'],
      })
    },
  })
  const labels: Record<string, string> = {
    members: t('Members'),
    logs: t('Usage Logs'),
    orders: t('Orders'),
    subscriptions: t('Subscriptions'),
    audit: t('Organization audit'),
    user_id: t('User ID'),
    role: t('Role'),
    status: t('Status'),
    spend_limit: t('Spending limit'),
    name: t('Name'),
    key: t('API Key'),
    used_quota: t('Used Quota'),
    created_at: t('Time'),
    model_name: t('Model'),
    quota: t('Quota'),
    request_id: t('Request ID'),
    trade_no: t('Order number'),
    plan_id: t('Plan ID'),
    money: t('Amount'),
    id: t('ID'),
    amount_total: t('Total Quota'),
    amount_used: t('Used Quota'),
    actor_id: t('Actor'),
    action: t('Action'),
    object_id: t('Object'),
    result: t('Result'),
    reason: t('Reason'),
  }
  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Organizations')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='space-y-5 px-4 pb-6'>
          <p className='text-muted-foreground'>
            {t('Platform administration')} ·{' '}
            {t('Global resources across organizations')}
          </p>
          <Input
            className='max-w-md'
            aria-label={t('Search organizations')}
            placeholder={t('Search organizations')}
            value={keyword}
            onChange={(event) => {
              setKeyword(event.target.value)
              setPage(1)
            }}
          />
          {organizations.isError ? (
            <Button
              onClick={() => {
                void organizations.refetch()
              }}
            >
              {t('Retry')}
            </Button>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Organization')}</TableHead>
                  <TableHead>{t('Type')}</TableHead>
                  <TableHead>{t('Owner')}</TableHead>
                  <TableHead>{t('Status')}</TableHead>
                  <TableHead>{t('Organization wallet')}</TableHead>
                  <TableHead>{t('Actions')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {organizations.data?.items.map((org) => (
                  <TableRow key={org.id}>
                    <TableCell>
                      {org.name}
                      <p className='text-muted-foreground text-xs'>
                        {org.slug}
                      </p>
                    </TableCell>
                    <TableCell>
                      {org.kind === 'personal' ? t('Personal') : t('Team')}
                    </TableCell>
                    <TableCell>{org.owner_id}</TableCell>
                    <TableCell>
                      {org.status === 1 ? t('Active') : t('Inactive')}
                    </TableCell>
                    <TableCell>{formatQuotaWithCurrency(org.quota)}</TableCell>
                    <TableCell>
                      <Button
                        variant='outline'
                        size='sm'
                        onClick={() => {
                          setSelected(org)
                          setResourcePage(1)
                        }}
                      >
                        {t('View details')}
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
          <div className='flex gap-2'>
            <Button
              variant='outline'
              disabled={page === 1}
              onClick={() => setPage(page - 1)}
            >
              {t('Previous')}
            </Button>
            <Button
              variant='outline'
              disabled={page * 20 >= (organizations.data?.total ?? 0)}
              onClick={() => setPage(page + 1)}
            >
              {t('Next')}
            </Button>
          </div>
          {selected && (
            <section className='space-y-4 rounded-xl border p-4'>
              <div className='flex flex-wrap items-center justify-between gap-3'>
                <h2 className='font-semibold'>{selected.name}</h2>
                <Button variant='outline' onClick={() => setConfirm(true)}>
                  {selected.status === 1
                    ? t('Disable organization')
                    : t('Restore organization')}
                </Button>
              </div>
              <Tabs
                value={resource}
                onValueChange={(value) => {
                  setResource(value as Resource)
                  setResourcePage(1)
                }}
              >
                <TabsList className='h-auto flex-wrap'>
                  {Object.keys(resourceColumns).map((key) => (
                    <TabsTrigger key={key} value={key}>
                      {labels[key]}
                    </TabsTrigger>
                  ))}
                </TabsList>
              </Tabs>
              {resources.isError ? (
                <Button
                  onClick={() => {
                    void resources.refetch()
                  }}
                >
                  {t('Retry')}
                </Button>
              ) : (
                <Table>
                  <TableHeader>
                    <TableRow>
                      {resourceColumns[resource].map((column) => (
                        <TableHead key={column}>{labels[column]}</TableHead>
                      ))}
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {resources.data?.items.map((row, index) => (
                      <TableRow key={row.id ?? index}>
                        {resourceColumns[resource].map((column) => (
                          <TableCell
                            key={column}
                            className='max-w-64 break-words'
                          >
                            {column === 'created_at' && row[column]
                              ? new Date(
                                  Number(row[column]) * 1000
                                ).toLocaleString()
                              : String(row[column] ?? '—')}
                          </TableCell>
                        ))}
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              )}
              <div className='flex gap-2'>
                <Button
                  variant='outline'
                  disabled={resourcePage === 1}
                  onClick={() => setResourcePage(resourcePage - 1)}
                >
                  {t('Previous')}
                </Button>
                <Button
                  variant='outline'
                  disabled={resourcePage * 20 >= (resources.data?.total ?? 0)}
                  onClick={() => setResourcePage(resourcePage + 1)}
                >
                  {t('Next')}
                </Button>
              </div>
            </section>
          )}
          <ConfirmDialog
            open={confirm}
            onOpenChange={setConfirm}
            title={t('Change organization status')}
            desc={t(
              'This action affects every member and API key in {{name}}.',
              { name: selected?.name }
            )}
            disabled={!reason.trim()}
            isLoading={status.isPending}
            handleConfirm={() => status.mutate()}
          >
            <Input
              aria-label={t('Reason')}
              placeholder={t('Reason')}
              value={reason}
              maxLength={256}
              onChange={(event) => setReason(event.target.value)}
            />
          </ConfirmDialog>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
