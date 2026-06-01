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
import { api } from '@/lib/api'
import type {
  AddMemberPayload,
  ApiResponse,
  ChatConversation,
  ChatMessage,
  ChatUser,
  CreateDirectConversationPayload,
  CreateGroupConversationPayload,
  MarkReadPayload,
  SendMessagePayload,
  ToggleMessageReactionPayload,
} from './types'

export const messagingQueryKeys = {
  users: ['messaging', 'users'] as const,
  conversations: ['messaging', 'conversations'] as const,
  messages: (conversationId: number) =>
    ['messaging', 'messages', conversationId] as const,
}

export async function listChatUsers(): Promise<ApiResponse<ChatUser[]>> {
  const res = await api.get('/api/chat/users')
  return res.data
}

export async function listChatConversations(): Promise<
  ApiResponse<ChatConversation[]>
> {
  const res = await api.get('/api/chat/conversations')
  return res.data
}

export async function createDirectConversation(
  payload: CreateDirectConversationPayload
): Promise<ApiResponse<ChatConversation>> {
  const res = await api.post('/api/chat/conversations/direct', payload)
  return res.data
}

export async function createGroupConversation(
  payload: CreateGroupConversationPayload
): Promise<ApiResponse<ChatConversation>> {
  const res = await api.post('/api/chat/conversations/group', payload)
  return res.data
}

export async function listChatMessages(
  conversationId: number,
  beforeMessageId = 0,
  limit = 50
): Promise<ApiResponse<ChatMessage[]>> {
  const params = new URLSearchParams()
  params.set('limit', String(limit))
  if (beforeMessageId > 0) params.set('before', String(beforeMessageId))
  const res = await api.get(
    `/api/chat/conversations/${conversationId}/messages?${params.toString()}`
  )
  return res.data
}

export async function sendChatMessage(
  conversationId: number,
  payload: SendMessagePayload
): Promise<ApiResponse<ChatMessage>> {
  const res = await api.post(
    `/api/chat/conversations/${conversationId}/messages`,
    payload
  )
  return res.data
}

export async function revokeChatMessage(
  conversationId: number,
  messageId: number
): Promise<ApiResponse<ChatMessage>> {
  const res = await api.post(
    `/api/chat/conversations/${conversationId}/messages/${messageId}/revoke`
  )
  return res.data
}

export async function toggleChatMessageReaction(
  conversationId: number,
  messageId: number,
  payload: ToggleMessageReactionPayload
): Promise<ApiResponse<ChatMessage>> {
  const res = await api.post(
    `/api/chat/conversations/${conversationId}/messages/${messageId}/reactions`,
    payload
  )
  return res.data
}

export async function markChatRead(
  conversationId: number,
  payload: MarkReadPayload
): Promise<ApiResponse<null>> {
  const res = await api.post(
    `/api/chat/conversations/${conversationId}/read`,
    payload
  )
  return res.data
}

export async function addChatMember(
  conversationId: number,
  payload: AddMemberPayload
): Promise<ApiResponse<null>> {
  const res = await api.post(
    `/api/chat/conversations/${conversationId}/members`,
    payload
  )
  return res.data
}

export async function removeChatMember(
  conversationId: number,
  userId: number
): Promise<ApiResponse<null>> {
  const res = await api.delete(
    `/api/chat/conversations/${conversationId}/members/${userId}`
  )
  return res.data
}
