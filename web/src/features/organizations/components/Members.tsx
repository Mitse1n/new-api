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
import { Search, UserPlus } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from '@/components/ui/empty'
import { Input } from '@/components/ui/input'
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from '@/components/ui/input-group'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatQuotaWithCurrency } from '@/lib/currency'

import {
  getOrganizationSummary,
  getOrganizationInvites,
  getOrganizationMembers,
  organizationMutation,
} from '../api'
import { useOrganization } from '../context'
import type { OrganizationMember } from '../types'
import { MemberDialog } from './MemberDialog'

export function Members(props: { budgets?: boolean }) {
  const { t } = useTranslation()
  const context = useOrganization()
  const client = useQueryClient()
  const manage = context.capabilities.org['org.member']?.write === true
  const team = context.organization.kind === 'team'
  const [resentLink, setResentLink] = useState('')
  const resend = useMutation({
    mutationFn: (id: number) =>
      organizationMutation<{ token: string }>('post', `invites/${id}/resend`),
    onSuccess: (data) => {
      setResentLink(
        `${window.location.origin}/organization/invite?token=${data.token}`
      )
      void client.invalidateQueries({ queryKey: ['organization-invites'] })
    },
  })
  const [search, setSearch] = useState('')
  const [dialog, setDialog] = useState<OrganizationMember | 'invite' | null>(
    null
  )
  const members = useQuery({
    queryKey: ['organization-members', context.organization.id],
    queryFn: getOrganizationMembers,
  })
  const summary = useQuery({
    queryKey: ['organization-summary', context.organization.id],
    queryFn: getOrganizationSummary,
    enabled: !!props.budgets,
  })
  const invites = useQuery({
    queryKey: ['organization-invites', context.organization.id],
    queryFn: getOrganizationInvites,
    enabled: manage && team && !props.budgets,
  })
  const revoke = useMutation({
    mutationFn: (id: number) => organizationMutation('delete', `invites/${id}`),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: ['organization-invites'] })
    },
  })
  const roleLabels = {
    owner: t('Owner'),
    admin: t('Admin'),
    member: t('Member'),
  }
  const inviteLabels = {
    pending: t('Pending'),
    accepted: t('Accepted'),
    expired: t('Expired'),
    revoked: t('Revoked'),
  }
  const filtered = (members.data ?? []).filter((member) =>
    `${member.email} ${member.username} ${member.display_name}`
      .toLowerCase()
      .includes(search.toLowerCase())
  )
  if (members.isError) {
    return (
      <Button
        variant='outline'
        onClick={() => {
          void members.refetch()
        }}
      >
        {t('Retry')}
      </Button>
    )
  }
  return (
    <div className='flex flex-col gap-5'>
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <InputGroup className='max-w-sm'>
          <InputGroupAddon>
            <Search />
          </InputGroupAddon>
          <InputGroupInput
            aria-label={t('Search members')}
            placeholder={t('Search members')}
            value={search}
            onChange={(event) => setSearch(event.target.value)}
          />
        </InputGroup>
        {manage && team && !props.budgets && (
          <Button onClick={() => setDialog('invite')}>
            <UserPlus />
            {t('Invite member')}
          </Button>
        )}
      </div>
      <Card>
        <CardHeader>
          <CardTitle>
            {props.budgets ? t('Member spending limits') : t('Members')}
          </CardTitle>
        </CardHeader>
        <CardContent>
          {members.isPending ? (
            <Skeleton className='h-40 w-full' />
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Member')}</TableHead>
                  <TableHead>{t('Role')}</TableHead>
                  <TableHead>{t('Status')}</TableHead>
                  <TableHead>{t('Spending limit')}</TableHead>
                  {props.budgets && (
                    <>
                      <TableHead>{t('Current period usage')}</TableHead>
                      <TableHead>{t('Pending reservations')}</TableHead>
                    </>
                  )}
                  <TableHead className='text-end'>{t('Actions')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {filtered.map((member) => (
                  <TableRow key={member.id}>
                    <TableCell>
                      <strong>{member.display_name || member.username}</strong>
                      <p className='text-muted-foreground text-xs'>
                        {member.email}
                      </p>
                    </TableCell>
                    <TableCell>
                      <Badge variant='secondary'>
                        {roleLabels[member.role]}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      {member.status === 1 ? t('Active') : t('Inactive')}
                    </TableCell>
                    <TableCell>
                      {member.spend_limit
                        ? formatQuotaWithCurrency(member.spend_limit)
                        : t('Unlimited')}
                    </TableCell>
                    {props.budgets && (
                      <>
                        <TableCell>
                          {formatQuotaWithCurrency(
                            summary.data?.usage.find(
                              (row) => row.user_id === member.user_id
                            )?.used ?? 0
                          )}
                        </TableCell>
                        <TableCell>
                          {formatQuotaWithCurrency(
                            summary.data?.usage.find(
                              (row) => row.user_id === member.user_id
                            )?.reserved ?? 0
                          )}
                        </TableCell>
                      </>
                    )}
                    <TableCell className='text-end'>
                      {manage &&
                        (props.budgets ||
                          (team && member.role !== 'owner')) && (
                          <Button
                            size='sm'
                            variant='ghost'
                            onClick={() => setDialog(member)}
                          >
                            {t('Edit')}
                          </Button>
                        )}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
          {!members.isPending && filtered.length === 0 && (
            <Empty>
              <EmptyHeader>
                <EmptyTitle>{t('No members found')}</EmptyTitle>
                <EmptyDescription>
                  {t('Try a different search.')}
                </EmptyDescription>
              </EmptyHeader>
            </Empty>
          )}
        </CardContent>
      </Card>
      {manage && team && !props.budgets && (
        <Card>
          <CardHeader>
            <CardTitle>{t('Invitations')}</CardTitle>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Username')}</TableHead>
                  <TableHead>{t('Role')}</TableHead>
                  <TableHead>{t('Status')}</TableHead>
                  <TableHead>{t('Expires')}</TableHead>
                  <TableHead className='text-end'>{t('Actions')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {(invites.data ?? []).map((invite) => (
                  <TableRow key={invite.id}>
                    <TableCell>
                      {invite.username || invite.email}
                      {!invite.invitee_id && (
                        <p className='text-muted-foreground text-xs'>
                          {t(
                            'Legacy email invitation. Revoke it and invite by username.'
                          )}
                        </p>
                      )}
                    </TableCell>
                    <TableCell>{roleLabels[invite.role]}</TableCell>
                    <TableCell>{inviteLabels[invite.status]}</TableCell>
                    <TableCell>
                      {new Date(invite.expires_at * 1000).toLocaleDateString()}
                    </TableCell>
                    <TableCell className='text-end'>
                      {!!invite.invitee_id &&
                        (invite.status === 'pending' ||
                          invite.status === 'expired') && (
                          <Button
                            size='sm'
                            variant='ghost'
                            disabled={resend.isPending}
                            onClick={() => resend.mutate(invite.id)}
                          >
                            {t('Resend')}
                          </Button>
                        )}
                      {invite.status === 'pending' && (
                        <Button
                          size='sm'
                          variant='ghost'
                          disabled={revoke.isPending}
                          onClick={() => revoke.mutate(invite.id)}
                        >
                          {t('Revoke')}
                        </Button>
                      )}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      )}
      <Dialog
        open={!!resentLink}
        onOpenChange={(open) => {
          if (!open) setResentLink('')
        }}
        title={t('Invitation link')}
        contentHeight='auto'
      >
        <p>{t('The previous invitation link is no longer valid.')}</p>
        <Input readOnly aria-label={t('Invitation link')} value={resentLink} />
        <Button
          onClick={() => {
            void navigator.clipboard
              .writeText(resentLink)
              .then(() => toast.success(t('Copied')))
              .catch(() => toast.error(t('Copy failed')))
          }}
        >
          {t('Copy')}
        </Button>
      </Dialog>
      {dialog && (
        <MemberDialog
          member={dialog === 'invite' ? undefined : dialog}
          budget={props.budgets}
          close={() => setDialog(null)}
        />
      )}
    </div>
  )
}
