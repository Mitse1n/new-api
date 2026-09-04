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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { SubscriptionPlansCard } from '@/features/wallet/components/subscription-plans-card'
import { useTopupInfo } from '@/features/wallet/hooks'
import { formatBillingCurrencyFromUSD } from '@/lib/currency'

import {
  getOrganizationAudit,
  getOrganizationOrders,
  getOrganizationSummary,
} from '../api'
import { useOrganization } from '../context'

export function PlansAndOrders() {
  const { t } = useTranslation()
  const paymentMethods: Record<string, string> = {
    balance: t('Balance'),
    stripe: 'Stripe',
    creem: 'Creem',
    waffo: 'Waffo',
    waffo_pancake: 'Waffo-Pancake',
    epay: t('Epay'),
  }
  const orderStatuses: Record<string, string> = {
    success: t('Paid'),
    pending: t('Pending'),
    failed: t('Failed'),
    expired: t('Expired'),
  }
  const context = useOrganization()
  const [page, setPage] = useState(1)
  const orders = useQuery({
    queryKey: ['organization-orders', context.organization.id, page],
    queryFn: () => getOrganizationOrders(page),
  })
  const summary = useQuery({
    queryKey: ['organization-summary', context.organization.id],
    queryFn: getOrganizationSummary,
  })
  const { topupInfo } = useTopupInfo()
  return (
    <div className='flex flex-col gap-5'>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Plans and prices are defined by the platform. Purchases belong to {{name}}.',
          { name: context.organization.name }
        )}
      </p>
      <SubscriptionPlansCard
        topupInfo={topupInfo}
        userQuota={summary.data?.quota}
        onPurchaseSuccess={() => {
          void orders.refetch()
          void summary.refetch()
        }}
      />
      <Card>
        <CardHeader>
          <CardTitle>{t('Order history')}</CardTitle>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('Order number')}</TableHead>
                <TableHead>{t('Plan')}</TableHead>
                <TableHead>{t('Amount')}</TableHead>
                <TableHead>{t('Payment method')}</TableHead>
                <TableHead>{t('Status')}</TableHead>
                <TableHead>{t('Time')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {(orders.data?.items ?? []).map((order) => (
                <TableRow key={order.id}>
                  <TableCell className='font-mono text-xs'>
                    {order.trade_no}
                  </TableCell>
                  <TableCell>
                    {order.plan_title || `#${order.plan_id}`}
                  </TableCell>
                  <TableCell>
                    {formatBillingCurrencyFromUSD(order.money)}
                  </TableCell>
                  <TableCell>
                    {paymentMethods[order.payment_method] ??
                      order.payment_method}
                  </TableCell>
                  <TableCell>
                    {orderStatuses[order.status] ?? t('Unknown')}
                  </TableCell>
                  <TableCell>
                    {new Date(order.create_time * 1000).toLocaleString()}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
          {orders.isError && (
            <Button
              onClick={() => {
                void orders.refetch()
              }}
            >
              {t('Retry')}
            </Button>
          )}
          <div className='mt-4 flex justify-end gap-2'>
            <Button
              variant='outline'
              disabled={page === 1}
              onClick={() => setPage(page - 1)}
            >
              {t('Previous')}
            </Button>
            <Button
              variant='outline'
              disabled={page * 20 >= (orders.data?.total ?? 0)}
              onClick={() => setPage(page + 1)}
            >
              {t('Next')}
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}

export function Audit() {
  const { t } = useTranslation()
  const context = useOrganization()
  const [page, setPage] = useState(1)
  const actions: Record<string, string> = {
    'organization.create': t('Create organization'),
    'request.failed': t('Request failed'),
    'subscription.checkout': t('Purchase Subscription'),
    'subscription.paid': t('Paid'),
    'organization.status': t('Organization status'),
    'platform.status': t('Organization status'),
    'member.invite': t('Invite member'),
    'member.accept': t('Accept invitation'),
    'member.update': t('Edit member'),
    'member.budget': t('Member spending limit'),
    'invite.revoke': t('Revoke invitation'),
    'invite.resend': t('Resend invitation'),
    'settings.update': t('Organization settings'),
    'ownership.request': t('Transfer ownership'),
    'ownership.accept': t('Accept ownership'),
    'token.create': t('Create API key'),
    'token.update': t('Edit API key'),
    'token.delete': t('Delete API key'),
  }
  const audit = useQuery({
    queryKey: ['organization-audit', context.organization.id, page],
    queryFn: () => getOrganizationAudit(page),
  })
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Organization audit')}</CardTitle>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Time')}</TableHead>
              <TableHead>{t('Actor')}</TableHead>
              <TableHead>{t('Action')}</TableHead>
              <TableHead>{t('Object')}</TableHead>
              <TableHead>{t('Result')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {(audit.data?.items ?? []).map((row) => (
              <TableRow key={row.id}>
                <TableCell>
                  {new Date(row.created_at * 1000).toLocaleString()}
                </TableCell>
                <TableCell>#{row.actor_id}</TableCell>
                <TableCell>{actions[row.action] ?? row.action}</TableCell>
                <TableCell>{row.object_id}</TableCell>
                <TableCell>
                  {row.result === 'success' ? t('Success') : t('Failed')}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
        <div className='mt-4 flex justify-end gap-2'>
          <Button
            variant='outline'
            disabled={page === 1}
            onClick={() => setPage(page - 1)}
          >
            {t('Previous')}
          </Button>
          <Button
            variant='outline'
            disabled={page * 20 >= (audit.data?.total ?? 0)}
            onClick={() => setPage(page + 1)}
          >
            {t('Next')}
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
