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
import { Link, Navigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'

import { Billing } from './components/Billing'
import { Members } from './components/Members'
import { Audit, PlansAndOrders } from './components/Orders'
import { OrganizationSummary } from './components/OrganizationSummary'
import { Settings } from './components/Settings'
import { useOrganization } from './context'

export function OrganizationPage(props: { section: string }) {
  const { t } = useTranslation()
  const context = useOrganization()
  let title = t('Members')
  let content = <Members />
  let permitted = true
  switch (props.section) {
    case 'billing':
      title = t('Billing & budgets')
      content = <Billing />
      break
    case 'plans':
      title = t('Plans & orders')
      content = <PlansAndOrders />
      permitted =
        context.capabilities.org['org.subscription']?.purchase === true
      break
    case 'settings':
      title = t('Organization settings')
      content = <Settings />
      permitted = context.capabilities.org['org.settings']?.write === true
      break
    case 'audit':
      title = t('Organization audit')
      content = <Audit />
      permitted = context.capabilities.org['org.usage']?.read_all === true
      break
    case 'members':
      break
    default:
      permitted = false
  }
  if (!permitted) {
    return (
      <Navigate to='/dashboard/$section' params={{ section: 'overview' }} />
    )
  }
  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{title}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='flex flex-col gap-5 px-4 pb-6'>
          <OrganizationSummary />
          {permitted ? (
            content
          ) : (
            <div className='flex flex-col items-start gap-3'>
              <p>{t('You do not have permission for this action.')}</p>
              <Button
                render={
                  <Link
                    to='/dashboard/$section'
                    params={{ section: 'overview' }}
                  />
                }
              >
                {t('Back to overview')}
              </Button>
            </div>
          )}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
