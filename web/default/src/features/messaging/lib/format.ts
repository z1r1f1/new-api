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
import type { ChatConversation, ChatMessage, ChatUser } from '../types'

export const MESSAGE_PAGE_SIZE = 40
export const MAX_MESSAGE_LENGTH = 2000

export type SidebarTab = 'users' | 'conversations'

export type MessageRow =
  | { kind: 'divider'; key: string; timestamp: number }
  | { kind: 'message'; key: string; message: ChatMessage }

export interface MessageReceiptSummary {
  readUsers: ChatUser[]
  unreadUsers: ChatUser[]
  totalRecipients: number
}

export function formatChatTime(timestamp: number): string {
  if (!timestamp) return '—'
  return new Intl.DateTimeFormat(undefined, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(timestamp * 1000))
}

export function formatChatDate(timestamp: number): string {
  if (!timestamp) return '—'
  return new Intl.DateTimeFormat(undefined, {
    year: 'numeric',
    month: 'short',
    day: '2-digit',
  }).format(new Date(timestamp * 1000))
}

export function getConversationTitle(conversation: ChatConversation): string {
  if (conversation.is_default) return 'Issue feedback group'
  if (conversation.type === 'direct' && conversation.peer) {
    return getChatUserDisplayName(conversation.peer)
  }
  if (conversation.title.trim()) return conversation.title
  if (conversation.type === 'direct') return 'Direct chat'
  return 'Group chat'
}

export function getConversationInitial(conversation: ChatConversation): string {
  const title = getConversationTitle(conversation).trim()
  if (!title) return '#'
  return title.slice(0, 1).toUpperCase()
}

export function mergeMessages(
  existing: ChatMessage[],
  incoming: ChatMessage[]
): ChatMessage[] {
  const byId = new Map<number, ChatMessage>()
  for (const message of existing) byId.set(message.id, message)
  for (const message of incoming) byId.set(message.id, message)
  return [...byId.values()].sort((a, b) => a.id - b.id)
}

export function buildMessageRows(messages: ChatMessage[]): MessageRow[] {
  const rows: MessageRow[] = []
  let currentBucket = ''
  for (const message of messages) {
    const bucket = getDateBucketKey(message.created_at)
    if (bucket !== currentBucket) {
      currentBucket = bucket
      rows.push({
        kind: 'divider',
        key: `divider-${bucket}-${message.id}`,
        timestamp: message.created_at,
      })
    }
    rows.push({ kind: 'message', key: `message-${message.id}`, message })
  }
  return rows
}

export function filterMessages(
  messages: ChatMessage[],
  searchText: string
): ChatMessage[] {
  const keyword = searchText.trim().toLowerCase()
  if (!keyword) return messages
  return messages.filter((message) => {
    return (
      message.body.toLowerCase().includes(keyword) ||
      message.sender_username.toLowerCase().includes(keyword) ||
      message.sender_display_name.toLowerCase().includes(keyword)
    )
  })
}

export function filterChatUsers(
  users: ChatUser[],
  searchText: string
): ChatUser[] {
  const keyword = searchText.trim().toLowerCase()
  if (!keyword) return users
  return users.filter((user) => {
    const displayName = getChatUserDisplayName(user).toLowerCase()
    return (
      displayName.includes(keyword) ||
      user.username.toLowerCase().includes(keyword)
    )
  })
}

export function filterConversations(
  conversations: ChatConversation[],
  searchText: string
): ChatConversation[] {
  const keyword = searchText.trim().toLowerCase()
  if (!keyword) return conversations
  return conversations.filter((conversation) => {
    return getConversationTitle(conversation).toLowerCase().includes(keyword)
  })
}

export function getInitialConversation(
  conversations: ChatConversation[]
): ChatConversation | null {
  const recentConversation = conversations.reduce<ChatConversation | null>(
    (latest, conversation) => {
      if (conversation.last_message_at <= 0) return latest
      if (!latest || conversation.last_message_at > latest.last_message_at) {
        return conversation
      }
      return latest
    },
    null
  )
  if (recentConversation) return recentConversation
  return conversations.find((conversation) => conversation.is_default) ?? null
}

export function getChatUserDisplayName(user: ChatUser): string {
  const displayName = user.display_name.trim()
  if (displayName) return displayName
  if (user.username.trim()) return user.username
  return 'User'
}

export function getChatUserInitial(user: ChatUser): string {
  const name = getChatUserDisplayName(user).trim()
  if (!name) return '#'
  return name.slice(0, 1).toUpperCase()
}

export function getMessageSenderName(message: ChatMessage): string {
  const displayName = message.sender_display_name.trim()
  if (displayName) return displayName
  if (message.sender_username.trim()) return message.sender_username
  return 'User'
}

export function getMessageReadLabel(
  message: ChatMessage,
  conversation: ChatConversation | null,
  currentUserId: number | null
): 'read' | 'unread' {
  if (!currentUserId) return 'unread'
  if (message.sender_id !== currentUserId) {
    if (!conversation) return 'unread'
    return conversation.last_read_message_id >= message.id ? 'read' : 'unread'
  }
  if (!conversation) return 'unread'
  const otherMembers = conversation.members.filter(
    (member) => member.id !== currentUserId
  )
  if (otherMembers.length === 0) return 'read'
  const readByIds = new Set(message.read_by.map((user) => user.id))
  return otherMembers.every((member) => readByIds.has(member.id))
    ? 'read'
    : 'unread'
}

export function getMessageReceiptSummary(
  message: ChatMessage,
  conversation: ChatConversation | null
): MessageReceiptSummary {
  if (!conversation) {
    return { readUsers: [], unreadUsers: [], totalRecipients: 0 }
  }
  const readByIds = new Set(message.read_by.map((user) => user.id))
  const recipients = conversation.members.filter(
    (member) => member.id !== message.sender_id
  )
  return {
    readUsers: recipients.filter((member) => readByIds.has(member.id)),
    unreadUsers: recipients.filter((member) => !readByIds.has(member.id)),
    totalRecipients: recipients.length,
  }
}

export function getTotalUnreadCount(conversations: ChatConversation[]): number {
  return conversations.reduce(
    (total, conversation) =>
      total + Math.max(0, conversation.unread_count || 0),
    0
  )
}

export function getDirectConversationKey(
  currentUserId: number | null,
  peerUserId: number
): string {
  if (!currentUserId || peerUserId <= 0) return ''
  const ids = [currentUserId, peerUserId].sort((a, b) => a - b)
  return `${ids[0]}:${ids[1]}`
}

export function getRealtimeBadgeVariant(
  status: string
): 'default' | 'outline' | 'secondary' {
  if (status === 'connected') return 'default'
  if (status === 'connecting') return 'secondary'
  return 'outline'
}

function getDateBucketKey(timestamp: number): string {
  if (!timestamp) return 'unknown'
  const date = new Date(timestamp * 1000)
  return `${date.getFullYear()}-${date.getMonth()}-${date.getDate()}`
}
