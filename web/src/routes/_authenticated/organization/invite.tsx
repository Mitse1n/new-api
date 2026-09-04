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
import { useMutation } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { acceptOrganizationInvite } from '@/features/organizations/api'
import { useSwitchOrganization } from '@/features/organizations/context'
import { useAuthStore } from '@/stores/auth-store'

export const Route = createFileRoute('/_authenticated/organization/invite')({
  validateSearch: z.object({ token: z.string().catch('') }),
  component: AcceptInvite,
})
function AcceptInvite() {
  const { t } = useTranslation()
  const search = Route.useSearch()
  const email = useAuthStore((state) => state.auth.user?.email)
  const switchOrg = useSwitchOrganization()
  const accept = useMutation({
    mutationFn: () => acceptOrganizationInvite(search.token),
    onSuccess: (orgID) => switchOrg(orgID),
  })
  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Organization invitation')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <Card className='m-4 max-w-xl'>
          <CardHeader>
            <CardTitle>{t('Join organization')}</CardTitle>
            <CardDescription>
              {t(
                'Accept with the email address that received this invitation.'
              )}{' '}
              · {email}
            </CardDescription>
          </CardHeader>
          <CardContent className='flex flex-col gap-4'>
            {accept.isError && (
              <p role='alert' className='text-destructive'>
                {t(
                  'Invitation unavailable. Check your email address or ask the organization administrator for a new link.'
                )}
              </p>
            )}
            <Button
              disabled={search.token.length !== 64 || accept.isPending}
              onClick={() => accept.mutate()}
            >
              {t('Accept invitation')}
            </Button>
          </CardContent>
        </Card>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
