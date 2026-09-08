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
import { create } from 'zustand'

import type { OrganizationContext } from '@/features/organizations/types'

type OrganizationState = {
  userID: number | null
  activeOrgID: number | null
  epoch: number
  context: OrganizationContext | null
  bindUser: (userID: number) => void
  select: (orgID: number | null) => void
  setContext: (context: OrganizationContext, epoch: number) => void
}

// Persist only the selected ID, separately for each global login identity.
// Roles, permissions, names and balances always come from the server.
export const useOrganizationStore = create<OrganizationState>((set, get) => ({
  userID: null,
  activeOrgID: null,
  epoch: 0,
  context: null,
  bindUser: (userID) => {
    if (get().userID === userID) return
    let selected: number | null = null
    try {
      const stored = Number(localStorage.getItem(`new-api:org:v1:${userID}`))
      if (Number.isSafeInteger(stored) && stored > 0) selected = stored
    } catch {
      /* Storage can be unavailable in private browsing. */
    }
    set({
      userID,
      activeOrgID: selected,
      context: null,
      epoch: get().epoch + 1,
    })
  },
  select: (activeOrgID) => {
    const state = get()
    try {
      const key = `new-api:org:v1:${state.userID}`
      if (activeOrgID) localStorage.setItem(key, String(activeOrgID))
      else localStorage.removeItem(key)
    } catch {
      /* The current session still works without persistence. */
    }
    set({ activeOrgID, context: null, epoch: state.epoch + 1 })
  },
  setContext: (context, epoch) => {
    if (
      get().epoch !== epoch ||
      context.organization.id !== get().activeOrgID
    ) {
      return
    }
    set({ context })
  },
}))
