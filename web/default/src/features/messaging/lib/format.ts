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
import type { ChatConversation, ChatMessage } from '../types'

export const MESSAGE_PAGE_SIZE = 40
export const MAX_MESSAGE_LENGTH = 2000
export const QUICK_REPLIES = [
  'Received, I will follow up shortly.',
  'Please add more context for the next step.',
  'This has been scheduled for review.',
]

export type ConversationFilter = 'all' | 'direct' | 'group'

export type MessageRow =
  | { kind: 'divider'; key: string; timestamp: number }
  | { kind: 'message'; key: string; message: ChatMessage }

export function parseMemberIds(value: string): number[] {
  return value
    .split(',')
    .map((item) => Number(item.trim()))
    .filter((item) => Number.isInteger(item) && item > 0)
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
  if (conversation.title.trim()) return conversation.title
  return ''
}

export function getConversationFallbackTitle(
  conversation: ChatConversation
): string {
  const explicitTitle = getConversationTitle(conversation)
  if (explicitTitle) return explicitTitle
  if (conversation.type === 'direct') return `Direct chat #${conversation.id}`
  return `Group chat #${conversation.id}`
}

export function getConversationInitial(conversation: ChatConversation): string {
  const title = getConversationFallbackTitle(conversation).trim()
  if (!title) return '#'
  return title.slice(0, 1).toUpperCase()
}

export function getConversationTone(conversation: ChatConversation): string {
  const tones = [
    'from-sky-500/80 to-cyan-500/80',
    'from-emerald-500/80 to-teal-500/80',
    'from-amber-500/80 to-orange-500/80',
    'from-violet-500/80 to-fuchsia-500/80',
    'from-slate-500/80 to-zinc-600/80',
  ]
  return tones[Math.abs(conversation.id) % tones.length]
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
  return messages.filter((message) =>
    message.body.toLowerCase().includes(keyword)
  )
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
