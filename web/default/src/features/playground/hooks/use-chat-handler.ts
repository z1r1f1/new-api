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
import { useCallback, useRef, useState } from 'react'
import { toast } from 'sonner'
import {
  createPlaygroundDebugId,
  getPlaygroundUpstreamRequest,
  getImageGenerationTask,
  sendChatCompletion,
  sendImageGeneration,
} from '../api'
import { DEBUG_TABS, MESSAGE_STATUS, ERROR_MESSAGES } from '../constants'
import {
  buildChatCompletionPayload,
  buildImageGenerationPayload,
  extractImageTaskId,
  getImageGenerationFailureMessage,
  getImageGenerationWaitMessage,
  imageResponseToMarkdown,
  imageTaskResultToMarkdown,
  isImageGenerationModel,
  isSuccessfulImageTaskStatus,
  isTerminalImageTaskStatus,
  updateAssistantMessageWithError,
  updateLastAssistantMessage,
  processStreamingContent,
  finalizeMessage,
} from '../lib'
import type {
  ImageGenerationRequest,
  ImageGenerationSubmitResponse,
  ImageGenerationTaskResponse,
  Message,
  PlaygroundConfig,
  ParameterEnabled,
  PlaygroundDebugData,
  PlaygroundDebugTab,
  PlaygroundRequestPayload,
} from '../types'
import { useStreamRequest } from './use-stream-request'

interface UseChatHandlerOptions {
  config: PlaygroundConfig
  parameterEnabled: ParameterEnabled
  onMessageUpdate: (updater: (prev: Message[]) => Message[]) => void
  onDebugUpdate: (updater: (prev: PlaygroundDebugData) => PlaygroundDebugData) => void
  onDebugTabChange: (tab: PlaygroundDebugTab) => void
}

/**
 * Hook for handling chat message sending and receiving
 */
export function useChatHandler({
  config,
  parameterEnabled,
  onMessageUpdate,
  onDebugUpdate,
  onDebugTabChange,
}: UseChatHandlerOptions) {
  const { sendStreamRequest, stopStream, isStreaming } = useStreamRequest()
  const [isImageGenerating, setIsImageGenerating] = useState(false)
  const imageAbortControllerRef = useRef<AbortController | null>(null)
  const streamResponseRef = useRef('')

  const getPayloadModel = useCallback(
    (payload: PlaygroundRequestPayload) => {
      const model = (payload as Record<string, unknown>).model
      return typeof model === 'string' ? model : config.model
    },
    [config.model]
  )

  const getPayloadStream = useCallback(
    (payload: PlaygroundRequestPayload) => {
      const stream = (payload as Record<string, unknown>).stream
      return typeof stream === 'boolean' ? stream : config.stream
    },
    [config.stream]
  )

  const getChatPayload = useCallback(
    (messages: Message[], overridePayload?: PlaygroundRequestPayload) => {
      return (
        overridePayload ||
        buildChatCompletionPayload(messages, config, parameterEnabled)
      )
    },
    [config, parameterEnabled]
  )

  const getImagePayload = useCallback(
    (messages: Message[], overridePayload?: PlaygroundRequestPayload) => {
      if (!overridePayload) {
        return buildImageGenerationPayload(messages, config)
      }

      const overrideRecord = overridePayload as Record<string, unknown>
      const prompt = String(overrideRecord.prompt || '').trim()
      if (!prompt) {
        return buildImageGenerationPayload(messages, {
          ...config,
          model: getPayloadModel(overridePayload),
        })
      }

      const sanitizedPayload = { ...overrideRecord }
      delete sanitizedPayload.messages
      delete sanitizedPayload.stream

      return {
        ...sanitizedPayload,
        model: getPayloadModel(overridePayload),
        group:
          typeof sanitizedPayload.group === 'string'
            ? sanitizedPayload.group
            : config.group,
        prompt,
      } as ImageGenerationRequest
    },
    [config, getPayloadModel]
  )

  const startDebugRequest = useCallback(
    (payload: PlaygroundRequestPayload, isStreamingRequest: boolean) => {
      streamResponseRef.current = ''
      onDebugUpdate((prev) => ({
        ...prev,
        gatewayRequest: payload,
        upstreamRequest: null,
        request: payload,
        response: null,
        sseMessages: [],
        timestamp: new Date().toISOString(),
        isStreaming: isStreamingRequest,
      }))
      onDebugTabChange(DEBUG_TABS.REQUEST)
    },
    [onDebugUpdate, onDebugTabChange]
  )

  const updateDebugUpstreamRequest = useCallback(
    (upstreamRequest: unknown) => {
      onDebugUpdate((prev) => ({
        ...prev,
        upstreamRequest,
        request: upstreamRequest,
      }))
    },
    [onDebugUpdate]
  )

  const fetchAndUpdateDebugUpstreamRequest = useCallback(
    async (debugId: string) => {
      const upstreamRequest = await getPlaygroundUpstreamRequest(debugId)
      if (upstreamRequest !== null) {
        updateDebugUpstreamRequest(upstreamRequest)
      }
    },
    [updateDebugUpstreamRequest]
  )

  const completeDebugResponse = useCallback(
    (response: unknown) => {
      onDebugUpdate((prev) => ({
        ...prev,
        response,
        isStreaming: false,
      }))
      onDebugTabChange(DEBUG_TABS.RESPONSE)
    },
    [onDebugUpdate, onDebugTabChange]
  )

  const appendDebugSseMessage = useCallback(
    (message: string) => {
      streamResponseRef.current += `${message}\n`
      onDebugUpdate((prev) => ({
        ...prev,
        sseMessages: [...prev.sseMessages, message],
      }))
    },
    [onDebugUpdate]
  )

  // Handle stream update
  const handleStreamUpdate = useCallback(
    (type: 'reasoning' | 'content', chunk: string) => {
      onMessageUpdate((prev) =>
        updateLastAssistantMessage(prev, (message) => {
          if (message.status === MESSAGE_STATUS.ERROR) return message

          if (type === 'reasoning') {
            // Direct API reasoning_content
            return {
              ...message,
              reasoning: {
                content: (message.reasoning?.content || '') + chunk,
                duration: 0,
              },
              isReasoningStreaming: true,
              status: MESSAGE_STATUS.STREAMING,
            }
          }

          // Content streaming: handle <think> tags
          return {
            ...processStreamingContent(message, chunk),
            status: MESSAGE_STATUS.STREAMING,
          }
        })
      )
    },
    [onMessageUpdate]
  )

  // Handle stream complete
  const handleStreamComplete = useCallback(() => {
    onMessageUpdate((prev) =>
      updateLastAssistantMessage(prev, (message) =>
        message.status === MESSAGE_STATUS.COMPLETE ||
        message.status === MESSAGE_STATUS.ERROR
          ? message
          : { ...finalizeMessage(message), status: MESSAGE_STATUS.COMPLETE }
      )
    )
  }, [onMessageUpdate])

  // Handle stream error
  const handleStreamError = useCallback(
    (error: string, errorCode?: string) => {
      toast.error(error)
      onMessageUpdate((prev) =>
        updateAssistantMessageWithError(prev, error, errorCode)
      )
    },
    [onMessageUpdate]
  )

  // Send streaming chat request
  const sendStreamingChat = useCallback(
    (messages: Message[], overridePayload?: PlaygroundRequestPayload) => {
      const payload = getChatPayload(messages, overridePayload)
      const debugId = createPlaygroundDebugId()
      startDebugRequest(payload, true)
      sendStreamRequest(
        payload,
        handleStreamUpdate,
        () => {
          completeDebugResponse(streamResponseRef.current.trim())
          handleStreamComplete()
        },
        (error, errorCode) => {
          completeDebugResponse({ error, errorCode })
          handleStreamError(error, errorCode)
        },
        appendDebugSseMessage,
        debugId,
        updateDebugUpstreamRequest
      )
    },
    [
      getChatPayload,
      startDebugRequest,
      sendStreamRequest,
      handleStreamUpdate,
      completeDebugResponse,
      handleStreamComplete,
      handleStreamError,
      appendDebugSseMessage,
      updateDebugUpstreamRequest,
    ]
  )

  // Send non-streaming chat request
  const sendNonStreamingChat = useCallback(
    async (messages: Message[], overridePayload?: PlaygroundRequestPayload) => {
      const payload = getChatPayload(messages, overridePayload)
      const debugId = createPlaygroundDebugId()
      startDebugRequest(payload, false)

      try {
        const result = await sendChatCompletion(payload, debugId)
        if (result.upstreamRequest !== null) {
          updateDebugUpstreamRequest(result.upstreamRequest)
        }
        const response = result.data
        completeDebugResponse(response)
        const choice = response.choices?.[0]
        if (!choice) return

        onMessageUpdate((prev) =>
          updateLastAssistantMessage(prev, (message) => ({
            ...finalizeMessage(
              {
                ...message,
                versions: [
                  {
                    ...message.versions[0],
                    content: choice.message?.content || '',
                  },
                ],
              },
              choice.message?.reasoning_content
            ),
            status: MESSAGE_STATUS.COMPLETE,
          }))
        )
      } catch (error: unknown) {
        await fetchAndUpdateDebugUpstreamRequest(debugId)
        const err = error as {
          response?: {
            data?: { message?: string; error?: { code?: string } }
          }
          message?: string
        }
        const errorMessage =
          err?.response?.data?.message ||
          err?.message ||
          ERROR_MESSAGES.API_REQUEST_ERROR
        const errorCode = err?.response?.data?.error?.code || undefined
        completeDebugResponse({ error: errorMessage, errorCode })
        handleStreamError(errorMessage, errorCode)
      }
    },
    [
      getChatPayload,
      startDebugRequest,
      completeDebugResponse,
      onMessageUpdate,
      handleStreamError,
      updateDebugUpstreamRequest,
      fetchAndUpdateDebugUpstreamRequest,
    ]
  )

  const updateImageGenerationMessage = useCallback(
    (
      taskId: string,
      taskData: ImageGenerationTaskResponse | ImageGenerationSubmitResponse,
      attempt: number,
      startedAt: number
    ) => {
      onMessageUpdate((prev) =>
        updateLastAssistantMessage(prev, (message) => ({
          ...message,
          versions: [
            {
              ...message.versions[0],
              content: getImageGenerationWaitMessage(
                taskId,
                taskData,
                attempt,
                startedAt
              ),
            },
          ],
          status: MESSAGE_STATUS.LOADING,
        }))
      )
    },
    [onMessageUpdate]
  )

  const completeImageGenerationMessage = useCallback(
    (content: string) => {
      onMessageUpdate((prev) =>
        updateLastAssistantMessage(prev, (message) => ({
          ...finalizeMessage({
            ...message,
            versions: [
              {
                ...message.versions[0],
                content,
              },
            ],
          }),
          status: MESSAGE_STATUS.COMPLETE,
        }))
      )
    },
    [onMessageUpdate]
  )

  const sleep = useCallback((ms: number, signal: AbortSignal) => {
    return new Promise<void>((resolve, reject) => {
      if (signal.aborted) {
        reject(signal.reason || new DOMException('Aborted', 'AbortError'))
        return
      }

      const timeoutId = window.setTimeout(resolve, ms)
      signal.addEventListener(
        'abort',
        () => {
          window.clearTimeout(timeoutId)
          reject(signal.reason || new DOMException('Aborted', 'AbortError'))
        },
        { once: true }
      )
    })
  }, [])

  const pollImageGenerationTask = useCallback(
    async (taskId: string, signal: AbortSignal, startedAt: number) => {
      const maxAttempts = 240

      for (let attempt = 0; attempt < maxAttempts; attempt += 1) {
        await sleep(5000, signal)

        const taskData = await getImageGenerationTask(taskId, signal)
        const status = String(taskData.status || '').toLowerCase()

        if (!isTerminalImageTaskStatus(status)) {
          updateImageGenerationMessage(taskId, taskData, attempt, startedAt)
          continue
        }

        if (isSuccessfulImageTaskStatus(status)) {
          return taskData
        }

        throw new Error(getImageGenerationFailureMessage(taskData))
      }

      throw new Error(ERROR_MESSAGES.IMAGE_GENERATION_TIMEOUT)
    },
    [sleep, updateImageGenerationMessage]
  )

  const isAbortError = useCallback((error: unknown) => {
    const err = error as { name?: string; code?: string; message?: string }
    return (
      err?.name === 'AbortError' ||
      err?.code === 'ERR_CANCELED' ||
      err?.message === 'canceled'
    )
  }, [])

  const sendImageGenerationChat = useCallback(
    async (messages: Message[], overridePayload?: PlaygroundRequestPayload) => {
      imageAbortControllerRef.current?.abort()

      const abortController = new AbortController()
      imageAbortControllerRef.current = abortController
      setIsImageGenerating(true)

      const payload = getImagePayload(messages, overridePayload)
      const debugId = createPlaygroundDebugId()
      const startedAt = Date.now()
      startDebugRequest(payload, false)

      try {
        const submitData = await sendImageGeneration(
          payload,
          abortController.signal,
          debugId
        )
        completeDebugResponse(submitData)
        const taskId = extractImageTaskId(submitData)

        if (taskId) {
          updateImageGenerationMessage(taskId, submitData, -1, startedAt)
          const taskResult = await pollImageGenerationTask(
            taskId,
            abortController.signal,
            startedAt
          )
          await fetchAndUpdateDebugUpstreamRequest(debugId)
          completeDebugResponse(taskResult)
          completeImageGenerationMessage(
            imageTaskResultToMarkdown(taskId, taskResult)
          )
          return
        }

        completeImageGenerationMessage(
          imageResponseToMarkdown(submitData) ||
            'Image generation completed, but the response did not include displayable image data.'
        )
      } catch (error: unknown) {
        if (isAbortError(error)) {
          return
        }
        await fetchAndUpdateDebugUpstreamRequest(debugId)

        const err = error as {
          response?: {
            data?: {
              message?: string
              error?: { code?: string; message?: string }
            }
          }
          message?: string
        }
        const errorMessage =
          err?.response?.data?.error?.message ||
          err?.response?.data?.message ||
          err?.message ||
          ERROR_MESSAGES.API_REQUEST_ERROR
        const errorCode = err?.response?.data?.error?.code || undefined
        completeDebugResponse({ error: errorMessage, errorCode })
        handleStreamError(errorMessage, errorCode)
      } finally {
        if (imageAbortControllerRef.current === abortController) {
          imageAbortControllerRef.current = null
        }
        setIsImageGenerating(false)
      }
    },
    [
      getImagePayload,
      startDebugRequest,
      completeDebugResponse,
      fetchAndUpdateDebugUpstreamRequest,
      updateImageGenerationMessage,
      pollImageGenerationTask,
      completeImageGenerationMessage,
      isAbortError,
      handleStreamError,
    ]
  )

  // Send chat request (stream or non-stream based on config)
  const sendChat = useCallback(
    (messages: Message[], overridePayload?: PlaygroundRequestPayload) => {
      const model = overridePayload
        ? getPayloadModel(overridePayload)
        : config.model
      if (isImageGenerationModel(model)) {
        sendImageGenerationChat(messages, overridePayload)
        return
      }

      const payload = getChatPayload(messages, overridePayload)
      if (getPayloadStream(payload)) {
        sendStreamingChat(messages, payload)
      } else {
        sendNonStreamingChat(messages, payload)
      }
    },
    [
      config.model,
      getChatPayload,
      getPayloadModel,
      getPayloadStream,
      sendImageGenerationChat,
      sendStreamingChat,
      sendNonStreamingChat,
    ]
  )

  // Stop generation
  const stopGeneration = useCallback(() => {
    imageAbortControllerRef.current?.abort()
    setIsImageGenerating(false)
    stopStream()
    onDebugUpdate((prev) => ({
      ...prev,
      response:
        prev.isStreaming && prev.response === null
          ? { status: 'stopped', response: streamResponseRef.current.trim() }
          : prev.response,
      isStreaming: false,
    }))
    onMessageUpdate((prev) =>
      updateLastAssistantMessage(prev, (message) =>
        message.status === MESSAGE_STATUS.LOADING ||
        message.status === MESSAGE_STATUS.STREAMING
          ? { ...finalizeMessage(message), status: MESSAGE_STATUS.COMPLETE }
          : message
      )
    )
  }, [stopStream, onDebugUpdate, onMessageUpdate])

  return {
    sendChat,
    stopGeneration,
    isGenerating: isStreaming || isImageGenerating,
  }
}
