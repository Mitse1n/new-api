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
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { DEFAULT_TIME_GRANULARITY } from '@/features/dashboard/constants'
import { useModelAnalytics } from '@/features/dashboard/hooks/use-model-analytics'
import type { UsageScope } from '@/features/dashboard/lib/usage-scope'
import type {
  DashboardChartPreferences,
  DashboardFilters,
} from '@/features/dashboard/types'

import { ConsumptionDistributionChart } from './consumption-distribution-chart'
import { LogStatCards } from './log-stat-cards'
import { ModelCharts } from './model-charts'

interface ModelAnalyticsProps {
  filters: DashboardFilters
  preferences: DashboardChartPreferences
  scope: UsageScope
}

export function ModelAnalytics(props: ModelAnalyticsProps) {
  const { t } = useTranslation()
  const { data, query, timeRange } = useModelAnalytics(
    props.filters,
    props.scope
  )
  return (
    <div className='space-y-6'>
      {query.isError && (
        <Alert variant='destructive'>
          <AlertTitle>{t('Request failed')}</AlertTitle>
          <AlertDescription>
            <Button
              variant='outline'
              size='sm'
              onClick={() => void query.refetch()}
            >
              {t('Retry')}
            </Button>
          </AlertDescription>
        </Alert>
      )}
      <LogStatCards
        data={data}
        loading={query.isPending || query.isFetching}
        error={query.isError}
        timeRangeMinutes={
          (timeRange.end_timestamp - timeRange.start_timestamp) / 60
        }
      />
      {!query.isError && (
        <>
          <ConsumptionDistributionChart
            data={data}
            loading={query.isPending || query.isFetching}
            defaultChartType={props.preferences.consumptionDistributionChart}
            timeGranularity={
              props.filters.time_granularity ?? DEFAULT_TIME_GRANULARITY
            }
          />
          <ModelCharts
            data={data}
            loading={query.isPending || query.isFetching}
            defaultChartTab={props.preferences.modelAnalyticsChart}
            timeGranularity={
              props.filters.time_granularity ?? DEFAULT_TIME_GRANULARITY
            }
          />
        </>
      )}
    </div>
  )
}
