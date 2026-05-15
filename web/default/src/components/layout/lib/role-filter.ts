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
import { ROLE } from '@/lib/roles'
import type { NavGroup, NavItem } from '../types'

function hasRequiredRole(
  requiredRole: number | undefined,
  userRole: number | undefined
): boolean {
  if (requiredRole === undefined) return true
  return (userRole ?? ROLE.GUEST) >= requiredRole
}

function filterNavItemByRole(
  item: NavItem,
  userRole: number | undefined
): NavItem | null {
  if (!hasRequiredRole(item.requiredRole, userRole)) return null

  if ('items' in item && item.items) {
    const filteredItems = item.items.filter((subItem) =>
      hasRequiredRole(subItem.requiredRole, userRole)
    )
    if (filteredItems.length === 0) return null
    return {
      ...item,
      items: filteredItems,
    }
  }

  return item
}

export function filterNavGroupsByRole(
  navGroups: NavGroup[],
  userRole: number | undefined
): NavGroup[] {
  return navGroups
    .filter((group) => hasRequiredRole(group.requiredRole, userRole))
    .map((group) => ({
      ...group,
      items: group.items
        .map((item) => filterNavItemByRole(item, userRole))
        .filter((item): item is NavItem => item !== null),
    }))
    .filter((group) => group.items.length > 0)
}
