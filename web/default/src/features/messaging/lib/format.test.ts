import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import type { ChatConversation, ChatMessage } from '../types'
import {
  getConversationTitle,
  getInitialConversation,
  mergeReactionEventMessage,
} from './format'

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

function message(
  id: number,
  overrides: Partial<ChatMessage> = {}
): ChatMessage {
  return {
    id,
    conversation_id: 1,
    sender_id: 1,
    sender_username: 'alice',
    sender_display_name: 'Alice',
    message_type: 'text',
    client_message_id: '',
    body: 'hello',
    created_at: 0,
    updated_at: 0,
    revoked_at: 0,
    revoked_by: 0,
    read_by: [],
    reactions: [],
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

describe('mergeReactionEventMessage', () => {
  test('preserves current user reaction flags when another user reacts', () => {
    const existing = message(1, {
      reactions: [{ emoji: '👍', count: 1, reacted_by_me: true }],
    })
    const incoming = message(1, {
      reactions: [{ emoji: '👍', count: 2, reacted_by_me: false }],
    })

    assert.deepEqual(
      mergeReactionEventMessage(existing, incoming, {
        currentUserId: 1,
        reactorUserId: 2,
        emoji: '👍',
        active: true,
      }).reactions,
      [{ emoji: '👍', count: 2, reacted_by_me: true }]
    )
  })

  test('applies current user reaction flag from realtime event metadata', () => {
    const incoming = message(1, {
      reactions: [{ emoji: '🎉', count: 1, reacted_by_me: false }],
    })

    assert.deepEqual(
      mergeReactionEventMessage(undefined, incoming, {
        currentUserId: 1,
        reactorUserId: 1,
        emoji: '🎉',
        active: true,
      }).reactions,
      [{ emoji: '🎉', count: 1, reacted_by_me: true }]
    )
  })
})
