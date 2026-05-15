import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { ROLE } from '@/lib/roles'
import type { NavGroup } from '../types'
import { filterNavGroupsByRole } from './role-filter'

const navGroups: NavGroup[] = [
  {
    id: 'general',
    title: 'General',
    items: [{ title: 'Overview', url: '/dashboard/overview' }],
  },
  {
    id: 'admin',
    title: 'Admin',
    requiredRole: ROLE.ADMIN,
    items: [
      { title: 'Channels', url: '/channels' },
      {
        title: 'System Settings',
        url: '/system-settings/site',
        requiredRole: ROLE.SUPER_ADMIN,
      },
    ],
  },
]

describe('filterNavGroupsByRole', () => {
  test('hides admin navigation from normal users', () => {
    const filtered = filterNavGroupsByRole(navGroups, ROLE.USER)

    assert.deepEqual(
      filtered.map((group) => group.id),
      ['general']
    )
  })

  test('keeps admin navigation but hides root-only items from admins', () => {
    const filtered = filterNavGroupsByRole(navGroups, ROLE.ADMIN)
    const adminGroup = filtered.find((group) => group.id === 'admin')

    assert.ok(adminGroup)
    assert.deepEqual(
      adminGroup.items.map((item) => item.title),
      ['Channels']
    )
  })

  test('keeps root-only items for super admins', () => {
    const filtered = filterNavGroupsByRole(navGroups, ROLE.SUPER_ADMIN)
    const adminGroup = filtered.find((group) => group.id === 'admin')

    assert.ok(adminGroup)
    assert.deepEqual(
      adminGroup.items.map((item) => item.title),
      ['Channels', 'System Settings']
    )
  })

  test('treats roles above super admin as root-capable', () => {
    const filtered = filterNavGroupsByRole(navGroups, ROLE.SUPER_ADMIN + 1)
    const adminGroup = filtered.find((group) => group.id === 'admin')

    assert.ok(adminGroup)
    assert.deepEqual(
      adminGroup.items.map((item) => item.title),
      ['Channels', 'System Settings']
    )
  })
})
