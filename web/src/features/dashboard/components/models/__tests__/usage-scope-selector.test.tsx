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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { useState } from 'react'
import { expect, test } from 'vitest'

import type { UsageScope } from '@/features/dashboard/lib/usage-scope'

import { UsageScopeSelector } from '../usage-scope-selector'

function SelectionFixture() {
  const [value, setValue] = useState<UsageScope>({ type: 'organization' })
  return (
    <UsageScopeSelector
      value={value}
      onChange={setValue}
      currentUserID={1}
      members={[
        { user_id: 2, username: 'alice', display_name: 'Alice' },
        { user_id: 1, username: 'viewer', display_name: 'Viewer' },
        { user_id: 3, username: 'bob', display_name: '' },
      ]}
    />
  )
}

test('Me is the first member and multiple members replace organization totals', async () => {
  render(<SelectionFixture />)
  const trigger = screen.getByRole('button', { name: 'Usage scope' })
  fireEvent.click(trigger)
  const organization = await screen.findByRole('menuitemcheckbox', {
    name: 'Organization total',
  })
  expect(organization).toHaveAttribute('aria-checked', 'true')
  expect(
    screen.getAllByRole('menuitemcheckbox').map((item) => item.textContent)
  ).toEqual(['Organization total', 'Me', 'Alice (@alice)', 'bob'])
  fireEvent.click(screen.getByRole('menuitemcheckbox', { name: 'Me' }))
  expect(organization).toHaveAttribute('aria-checked', 'false')
  expect(screen.getByRole('menuitemcheckbox', { name: 'Me' })).toHaveAttribute(
    'aria-disabled',
    'true'
  )
  fireEvent.click(
    screen.getByRole('menuitemcheckbox', { name: 'Alice (@alice)' })
  )
  expect(trigger).toHaveAttribute('aria-expanded', 'true')
  expect(trigger).toHaveTextContent('Me / Alice (@alice)')
  fireEvent.click(screen.getByRole('menuitemcheckbox', { name: 'Me' }))
  expect(trigger).toHaveTextContent('Alice (@alice)')
  fireEvent.click(organization)
  expect(trigger).toHaveTextContent('Organization total')
  expect(
    screen.getByRole('menuitemcheckbox', { name: 'Alice (@alice)' })
  ).toHaveAttribute('aria-checked', 'false')
})

test('keyboard users can open the selector and Escape restores trigger focus', async () => {
  render(<SelectionFixture />)
  const trigger = screen.getByRole('button', { name: 'Usage scope' })
  trigger.focus()
  fireEvent.keyDown(trigger, { key: 'ArrowDown' })
  await screen.findByRole('menu')
  fireEvent.keyDown(screen.getByRole('menu'), { key: 'Escape' })
  await waitFor(() => expect(trigger).toHaveAttribute('aria-expanded', 'false'))
  await waitFor(() => expect(trigger).toHaveFocus())
})
