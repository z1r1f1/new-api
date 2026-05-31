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
export type ChatConversationType = 'direct' | 'group'

export interface ChatUser {
  id: number
  username: string
  display_name: string
  role: number
}

export interface ChatConversation {
  id: number
  type: ChatConversationType
  title: string
  owner_id: number
  direct_key: string
  last_message_id: number
  last_message_at: number
  created_at: number
  updated_at: number
  unread_count: number
  last_read_message_id: number
  is_default: boolean
  peer?: ChatUser
  members: ChatUser[]
}

export interface ChatMessage {
  id: number
  conversation_id: number
  sender_id: number
  sender_username: string
  sender_display_name: string
  message_type: 'text'
  client_message_id: string
  body: string
  created_at: number
  updated_at: number
  read_by: ChatUser[]
}

export interface ChatEvent {
  type: string
  conversation_id?: number
  conversation?: ChatConversation
  message?: ChatMessage
  user_id?: number
  member_ids?: number[]
  last_read_message_id?: number
  created_at?: number
}

export interface ApiResponse<T> {
  success: boolean
  message?: string
  data?: T
}

export interface CreateDirectConversationPayload {
  user_id: number
}

export interface CreateGroupConversationPayload {
  title: string
  member_ids: number[]
}

export interface SendMessagePayload {
  body: string
  client_message_id: string
}

export interface MarkReadPayload {
  last_read_message_id: number
}

export interface AddMemberPayload {
  user_id: number
}
