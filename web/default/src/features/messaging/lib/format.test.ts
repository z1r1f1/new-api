import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import type { ChatConversation } from '../types'
import { getConversationTitle, getInitialConversation } from './format'

function conversation(
  id: number,
  overrides: Partial<ChatConversation> = {}
): ChatConversation {
  return {
    id,
    type: 'direct',
    title: '',
    owner_id: 1,
    direct_key: '',
    last_message_id: 0,
    last_message_at: 0,
    created_at: 0,
    updated_at: 0,
    unread_count: 0,
    last_read_message_id: 0,
    is_default: false,
    members: [],
    ...overrides,
  }
}

describe('getInitialConversation', () => {
  test('selects the most recently used conversation before the default group', () => {
    const defaultGroup = conversation(1, {
      type: 'group',
      title: 'Default group',
      is_default: true,
    })
    const olderDirect = conversation(2, { last_message_at: 100 })
    const recentDirect = conversation(3, { last_message_at: 200 })

    assert.equal(
      getInitialConversation([olderDirect, defaultGroup, recentDirect]),
      recentDirect
    )
  })

  test('falls back to the default group when there are no recently used conversations', () => {
    const emptyDirect = conversation(1)
    const defaultGroup = conversation(2, {
      type: 'group',
      title: 'Default group',
      is_default: true,
    })

    assert.equal(
      getInitialConversation([emptyDirect, defaultGroup]),
      defaultGroup
    )
  })

  test('returns null when no conversation can be selected', () => {
    assert.equal(getInitialConversation([]), null)
    assert.equal(getInitialConversation([conversation(1)]), null)
  })
})

describe('getConversationTitle', () => {
  test('labels the default group as the issue feedback group', () => {
    assert.equal(
      getConversationTitle(
        conversation(1, {
          type: 'group',
          title: 'Default group',
          is_default: true,
        })
      ),
      'Issue feedback group'
    )
  })
})
