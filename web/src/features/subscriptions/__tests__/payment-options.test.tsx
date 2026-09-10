import { render, screen, cleanup } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, expect, test } from 'vitest'

import { SubscriptionPurchaseDialog } from '../components/dialogs/subscription-purchase-dialog'
import { subscriptionPlanSchema } from '../types'

const i18n = createInstance()
await i18n.use(initReactI18next).init({ lng: 'en', resources: {} })
afterEach(cleanup)

test('subscription checkout excludes standalone Waffo while retaining supported providers', () => {
  const plan = {
    plan: subscriptionPlanSchema.parse({
      id: 1,
      title: 'Monthly',
      price_amount: 10,
      stripe_price_id: 'price_monthly',
      waffo_pancake_product_id: 'pancake_monthly',
      duration_unit: 'month',
      duration_value: 1,
      quota_reset_period: 'never',
      enabled: true,
      sort_order: 0,
      max_purchase_per_user: 0,
      total_amount: 100,
    }),
  }
  render(
    <I18nextProvider i18n={i18n}>
      <SubscriptionPurchaseDialog
        {...{ enableWaffo: true }}
        open
        onOpenChange={() => {}}
        plan={plan}
        enableStripe
        enableWaffoPancake
      />
    </I18nextProvider>
  )
  expect(
    screen.queryByRole('button', { name: 'Waffo' })
  ).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Stripe' })).toBeVisible()
  expect(screen.getByRole('button', { name: 'Waffo Pancake' })).toBeVisible()
})
