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
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'
import { formatQuotaWithCurrency } from '@/lib/currency'

import { getOrganizationSummary } from '../api'
import { useOrganization } from '../context'
import { Members } from './Members'

export function Billing() {
  const { t } = useTranslation()
  const context = useOrganization()
  const summary = useQuery({
    queryKey: ['organization-summary', context.organization.id],
    queryFn: getOrganizationSummary,
  })
  const data = summary.data
  if (!data) return <p role='status'>{t('Loading...')}</p>
  if (!context.capabilities.org['org.billing']?.read) {
    const own = data.usage.find(
      (row) => row.user_id === context.membership.user_id
    )
    return (
      <div className='space-y-5'>
        <Card>
          <CardHeader>
            <CardTitle>{t('My remaining limit')}</CardTitle>
            <CardDescription>
              {formatQuotaWithCurrency(data.available_quota)}
            </CardDescription>
          </CardHeader>
          <CardContent>
            <p>
              {t('Current period usage')}:{' '}
              {formatQuotaWithCurrency(own?.used ?? 0)}
            </p>
            <p>
              {t('Pending reservations')}:{' '}
              {formatQuotaWithCurrency(own?.reserved ?? 0)}
            </p>
          </CardContent>
        </Card>
        <Members budgets />
      </div>
    )
  }
  const active = data.subscriptions.filter(
    (sub) => sub.status === 'active' && sub.end_time > Date.now() / 1000
  )
  const remaining = active.reduce(
    (sum, sub) => sum + Math.max(0, sub.amount_total - sub.amount_used),
    0
  )
  const used = data.usage.reduce((sum, row) => sum + row.used, 0)
  const reserved = data.usage.reduce((sum, row) => sum + row.reserved, 0)
  const percent = data.budget_limit
    ? Math.min(100, ((used + reserved) / data.budget_limit) * 100)
    : 0
  return (
    <div className='flex flex-col gap-5'>
      <Alert>
        <AlertDescription>
          {t(
            'Members share the organization balance. Spending limits do not allocate separate wallets.'
          )}
        </AlertDescription>
      </Alert>
      <div className='grid gap-4 md:grid-cols-3'>
        <Card>
          <CardHeader>
            <CardDescription>{t('Organization wallet')}</CardDescription>
            <CardTitle className='text-2xl'>
              {formatQuotaWithCurrency(data.quota)}
            </CardTitle>
          </CardHeader>
          <CardContent>
            {context.capabilities.org['org.billing']?.write && (
              <Button variant='outline' render={<Link to='/wallet' />}>
                {t('Top up')}
              </Button>
            )}
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardDescription>{t('Subscription remaining')}</CardDescription>
            <CardTitle className='text-2xl'>
              {active.some((sub) => sub.amount_total === 0)
                ? t('Unlimited')
                : formatQuotaWithCurrency(remaining)}
            </CardTitle>
          </CardHeader>
          <CardContent className='text-muted-foreground text-sm'>
            {t(
              'Subscription quota is used first. Wallet fallback follows the purchased plan.'
            )}
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardDescription>{t('Current period usage')}</CardDescription>
            <CardTitle className='text-2xl'>
              {formatQuotaWithCurrency(used)}
            </CardTitle>
          </CardHeader>
          <CardContent className='text-muted-foreground text-sm'>
            {t('Pending reservations')}: {formatQuotaWithCurrency(reserved)}
          </CardContent>
        </Card>
      </div>
      <Card>
        <CardHeader>
          <CardTitle>{t('Budget period')}</CardTitle>
          <CardDescription>
            {data.period_start
              ? `${new Date(data.period_start * 1000).toLocaleDateString()} — ${new Date(data.period_end * 1000).toLocaleDateString()}`
              : t('The period starts with your first request.')}
          </CardDescription>
        </CardHeader>
        <CardContent className='flex flex-col gap-3'>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Member limits reset with the subscription allowance. Without a subscription, they reset each calendar month.'
            )}
          </p>
          {data.budget_limit > 0 && (
            <>
              <Progress value={percent} aria-label={t('Budget usage')} />
              <p>
                {formatQuotaWithCurrency(used)} /{' '}
                {formatQuotaWithCurrency(data.budget_limit)}
              </p>
            </>
          )}
        </CardContent>
      </Card>
      <Members budgets />
    </div>
  )
}
