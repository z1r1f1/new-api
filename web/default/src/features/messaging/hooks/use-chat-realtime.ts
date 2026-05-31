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
import { useEffect, useMemo, useState } from 'react'
import { Centrifuge, type PublicationContext } from 'centrifuge'
import type { ChatEvent } from '../types'

interface UseChatRealtimeOptions {
  userId: number | null
  conversationId: number | null
  onEvent: (event: ChatEvent) => void
}

function getChatWebSocketUrl(): string {
  if (typeof window === 'undefined') return '/api/chat/ws'
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  return `${protocol}//${window.location.host}/api/chat/ws`
}

function isChatEvent(value: unknown): value is ChatEvent {
  if (!value || typeof value !== 'object') return false
  const record = value as Record<string, unknown>
  return typeof record.type === 'string'
}

export function useChatRealtime(options: UseChatRealtimeOptions): string {
  const [status, setStatus] = useState('disconnected')
  const wsUrl = useMemo(() => getChatWebSocketUrl(), [])
  const userId = options.userId
  const conversationId = options.conversationId
  const onEvent = options.onEvent

  useEffect(() => {
    if (!userId) {
      return undefined
    }

    const client = new Centrifuge(wsUrl)
    const handlePublication = (ctx: PublicationContext): void => {
      if (isChatEvent(ctx.data)) {
        onEvent(ctx.data)
      }
    }

    client.on('connecting', () => setStatus('connecting'))
    client.on('connected', () => setStatus('connected'))
    client.on('disconnected', () => setStatus('disconnected'))

    const userSubscription = client.newSubscription(`user:${userId}`)
    userSubscription.on('publication', handlePublication)
    userSubscription.subscribe()

    const conversationSubscription = conversationId
      ? client.newSubscription(`conversation:${conversationId}`)
      : null
    if (conversationSubscription) {
      conversationSubscription.on('publication', handlePublication)
      conversationSubscription.subscribe()
    }

    client.connect()

    return () => {
      userSubscription.unsubscribe()
      if (conversationSubscription) conversationSubscription.unsubscribe()
      client.disconnect()
    }
  }, [conversationId, onEvent, userId, wsUrl])

  return userId ? status : 'disconnected'
}
