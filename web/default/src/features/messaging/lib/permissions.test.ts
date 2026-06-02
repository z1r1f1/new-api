import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { ROLE } from '@/lib/roles'
import type { ChatConversation, ChatUser } from '../types'
import {
  canManageChatProfileUser,
  getConversationMemberMuteState,
} from './permissions'

function user(id: number, overrides: Partial<ChatUser> = {}): ChatUser {
  return {
    id,
    username: `user-${id}`,
    display_name: `User ${id}`,
    role: ROLE.USER,
    ...overrides,
  }
}

function groupConversation(members: ChatUser[]): ChatConversation {
  return {
    id: 1,
    type: 'group',
    title: 'team',
    owner_id: 1,
    direct_key: '',
    last_message_id: 0,
    last_message_at: 0,
    created_at: 0,
    updated_at: 0,
    unread_count: 0,
    last_read_message_id: 0,
    is_default: false,
    members,
  }
}

describe('chat profile permissions', () => {
  test('allows root or higher-role admins to manage lower-role users', () => {
    assert.equal(canManageChatProfileUser(ROLE.ADMIN, ROLE.USER), true)
    assert.equal(canManageChatProfileUser(ROLE.SUPER_ADMIN, ROLE.ADMIN), true)
    assert.equal(canManageChatProfileUser(ROLE.ADMIN, ROLE.ADMIN), false)
    assert.equal(canManageChatProfileUser(ROLE.USER, ROLE.USER), false)
  })

  test('reads per-conversation mute state from group members only', () => {
    const muted = user(2, { muted: true })
    const conversation = groupConversation([user(1), muted])

    assert.deepEqual(getConversationMemberMuteState(conversation, 2), {
      isGroupMember: true,
      muted: true,
    })
    assert.deepEqual(getConversationMemberMuteState(conversation, 3), {
      isGroupMember: false,
      muted: false,
    })
    assert.deepEqual(getConversationMemberMuteState(null, 2), {
      isGroupMember: false,
      muted: false,
    })
  })
})
