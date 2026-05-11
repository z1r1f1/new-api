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
  getImageGenerationTask,
  sendChatCompletion,
  sendImageGeneration,
} from '../api'
import { MESSAGE_STATUS, ERROR_MESSAGES } from '../constants'
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
  ImageGenerationSubmitResponse,
  ImageGenerationTaskResponse,
  Message,
  PlaygroundConfig,
  ParameterEnabled,
} from '../types'
import { useStreamRequest } from './use-stream-request'

interface UseChatHandlerOptions {
  config: PlaygroundConfig
  parameterEnabled: ParameterEnabled
  onMessageUpdate: (updater: (prev: Message[]) => Message[]) => void
}

/**
 * Hook for handling chat message sending and receiving
 */
export function useChatHandler({
  config,
  parameterEnabled,
  onMessageUpdate,
}: UseChatHandlerOptions) {
  const { sendStreamRequest, stopStream, isStreaming } = useStreamRequest()
  const [isImageGenerating, setIsImageGenerating] = useState(false)
  const imageAbortControllerRef = useRef<AbortController | null>(null)

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
    (messages: Message[]) => {
      const payload = buildChatCompletionPayload(
        messages,
        config,
        parameterEnabled
      )
      sendStreamRequest(
        payload,
        handleStreamUpdate,
        handleStreamComplete,
        handleStreamError
      )
    },
    [
      config,
      parameterEnabled,
      sendStreamRequest,
      handleStreamUpdate,
      handleStreamComplete,
      handleStreamError,
    ]
  )

  // Send non-streaming chat request
  const sendNonStreamingChat = useCallback(
    async (messages: Message[]) => {
      const payload = buildChatCompletionPayload(
        messages,
        config,
        parameterEnabled
      )

      try {
        const response = await sendChatCompletion(payload)
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
        const err = error as {
          response?: {
            data?: { message?: string; error?: { code?: string } }
          }
          message?: string
        }
        handleStreamError(
          err?.response?.data?.message ||
            err?.message ||
            ERROR_MESSAGES.API_REQUEST_ERROR,
          err?.response?.data?.error?.code || undefined
        )
      }
    },
    [config, parameterEnabled, onMessageUpdate, handleStreamError]
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
    async (messages: Message[]) => {
      imageAbortControllerRef.current?.abort()

      const abortController = new AbortController()
      imageAbortControllerRef.current = abortController
      setIsImageGenerating(true)

      const payload = buildImageGenerationPayload(messages, config)
      const startedAt = Date.now()

      try {
        const submitData = await sendImageGeneration(
          payload,
          abortController.signal
        )
        const taskId = extractImageTaskId(submitData)

        if (taskId) {
          updateImageGenerationMessage(taskId, submitData, -1, startedAt)
          const taskResult = await pollImageGenerationTask(
            taskId,
            abortController.signal,
            startedAt
          )
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

        const err = error as {
          response?: {
            data?: {
              message?: string
              error?: { code?: string; message?: string }
            }
          }
          message?: string
        }
        handleStreamError(
          err?.response?.data?.error?.message ||
            err?.response?.data?.message ||
            err?.message ||
            ERROR_MESSAGES.API_REQUEST_ERROR,
          err?.response?.data?.error?.code || undefined
        )
      } finally {
        if (imageAbortControllerRef.current === abortController) {
          imageAbortControllerRef.current = null
        }
        setIsImageGenerating(false)
      }
    },
    [
      config,
      updateImageGenerationMessage,
      pollImageGenerationTask,
      completeImageGenerationMessage,
      isAbortError,
      handleStreamError,
    ]
  )

  // Send chat request (stream or non-stream based on config)
  const sendChat = useCallback(
    (messages: Message[]) => {
      if (isImageGenerationModel(config.model)) {
        sendImageGenerationChat(messages)
        return
      }

      if (config.stream) {
        sendStreamingChat(messages)
      } else {
        sendNonStreamingChat(messages)
      }
    },
    [
      config.model,
      config.stream,
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
    onMessageUpdate((prev) =>
      updateLastAssistantMessage(prev, (message) =>
        message.status === MESSAGE_STATUS.LOADING ||
        message.status === MESSAGE_STATUS.STREAMING
          ? { ...finalizeMessage(message), status: MESSAGE_STATUS.COMPLETE }
          : message
      )
    )
  }, [stopStream, onMessageUpdate])

  return {
    sendChat,
    stopGeneration,
    isGenerating: isStreaming || isImageGenerating,
  }
}
