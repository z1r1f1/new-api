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
import { useCallback } from 'react'
import { SSE } from 'sse.js'
import { getCommonHeaders } from '@/lib/api'
import { getPlaygroundDebugHeaders, getPlaygroundUpstreamRequest } from '../api'
import { API_ENDPOINTS, ERROR_MESSAGES } from '../constants'
import type { ChatCompletionChunk, PlaygroundRequestPayload } from '../types'

let activePlaygroundSseSource: SSE | null = null

/**
 * Hook for handling streaming chat completion requests
 */
export function useStreamRequest() {
  const sendStreamRequest = useCallback(
    (
      payload: PlaygroundRequestPayload,
      onUpdate: (type: 'reasoning' | 'content', chunk: string) => void,
      onComplete: () => void,
      onError: (error: string, errorCode?: string) => void,
      onRawMessage?: (message: string) => void,
      debugId?: string,
      onUpstreamRequest?: (request: unknown) => void
    ) => {
      activePlaygroundSseSource?.close()

      const source = new SSE(API_ENDPOINTS.CHAT_COMPLETIONS, {
        headers: {
          ...getCommonHeaders(),
          ...getPlaygroundDebugHeaders(debugId),
        },
        method: 'POST',
        payload: JSON.stringify(payload),
        start: false,
      })

      activePlaygroundSseSource = source
      let isStreamComplete = false

      const closeSource = () => {
        source.close()
        if (activePlaygroundSseSource === source) {
          activePlaygroundSseSource = null
        }
      }

      const handleError = (errorMessage: string, errorCode?: string) => {
        if (activePlaygroundSseSource === source && !isStreamComplete) {
          onError(errorMessage, errorCode)
          closeSource()
        }
      }

      let hasCapturedUpstreamRequest = false
      const captureUpstreamRequest = async () => {
        if (hasCapturedUpstreamRequest) {
          return
        }
        const upstreamRequest = await getPlaygroundUpstreamRequest(debugId)
        if (upstreamRequest !== null) {
          hasCapturedUpstreamRequest = true
          onUpstreamRequest?.(upstreamRequest)
        }
      }

      source.addEventListener('open', () => {
        void captureUpstreamRequest()
      })

      source.addEventListener('message', (e: MessageEvent) => {
        void captureUpstreamRequest()
        onRawMessage?.(e.data)
        if (e.data.trim() === '[DONE]') {
          isStreamComplete = true
          closeSource()
          onComplete()
          return
        }

        try {
          const chunk: ChatCompletionChunk = JSON.parse(e.data)
          const delta = chunk.choices?.[0]?.delta

          if (delta) {
            if (delta.reasoning_content) {
              onUpdate('reasoning', delta.reasoning_content)
            }
            if (delta.reasoning) {
              onUpdate('reasoning', delta.reasoning)
            }
            if (delta.content) {
              onUpdate('content', delta.content)
            }
          }
        } catch (error) {
          // eslint-disable-next-line no-console
          console.error('Failed to parse SSE message:', error)
          handleError(ERROR_MESSAGES.PARSE_ERROR)
        }
      })

      source.addEventListener('error', (e: Event & { data?: string }) => {
        // Only handle errors if stream didn't complete normally
        if (source.readyState !== 2) {
          // eslint-disable-next-line no-console
          console.error('SSE Error:', e)
          let errorMessage = e.data || ERROR_MESSAGES.API_REQUEST_ERROR
          let errorCode: string | undefined
          if (e.data) {
            try {
              const parsed = JSON.parse(e.data) as {
                error?: { message?: string; code?: string }
              }
              if (parsed?.error) {
                errorMessage = parsed.error.message || errorMessage
                errorCode = parsed.error.code || undefined
              }
            } catch {
              // not JSON, use raw string
            }
          }
          handleError(errorMessage, errorCode)
        }
      })

      source.addEventListener(
        'readystatechange',
        (e: Event & { readyState?: number }) => {
          const status = (source as unknown as { status?: number }).status
          if (
            e.readyState !== undefined &&
            e.readyState >= 2 &&
            status !== undefined &&
            status !== 200
          ) {
            handleError(`HTTP ${status}: ${ERROR_MESSAGES.CONNECTION_CLOSED}`)
          }
        }
      )

      try {
        source.stream()
      } catch (error: unknown) {
        // eslint-disable-next-line no-console
        console.error('Failed to start SSE stream:', error)
        onError(ERROR_MESSAGES.STREAM_START_ERROR)
        if (activePlaygroundSseSource === source) {
          activePlaygroundSseSource = null
        }
      }
    },
    []
  )

  const stopStream = useCallback(() => {
    if (activePlaygroundSseSource) {
      activePlaygroundSseSource.close()
      activePlaygroundSseSource = null
    }
  }, [])

  const isStreaming = activePlaygroundSseSource !== null

  return {
    sendStreamRequest,
    stopStream,
    isStreaming,
  }
}
