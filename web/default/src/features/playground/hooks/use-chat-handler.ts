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
import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
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
  extractImageGenerationWaitTaskId,
  extractImageTaskId,
  getImageGenerationFailureMessage,
  getImageGenerationWaitMessage,
  imageResponseToMarkdown,
  imageTaskResultToMarkdown,
  isImageGenerationRequestPayload,
  shouldUseImageGenerationPayload,
  isSuccessfulImageTaskStatus,
  isTerminalImageTaskStatus,
  loadPendingImageTasks,
  clearActivePlaygroundChatMessage,
  markActivePlaygroundChatMessage,
  removePendingImageTask,
  updateAssistantMessageWithError,
  updateLastAssistantMessage,
  processStreamingContent,
  finalizeMessage,
  upsertPendingImageTask,
  type PlaygroundStorageScope,
} from '../lib'
import type {
  ImageGenerationRequest,
  ImageGenerationSubmitResponse,
  ImageGenerationTaskResponse,
  Message,
  PendingImageGenerationTask,
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
  messages: Message[]
  activeSessionId: string
  storageUserId?: PlaygroundStorageScope
  onMessageUpdate: (updater: (prev: Message[]) => Message[]) => void
  onSessionMessageUpdate: (
    sessionId: string,
    updater: (prev: Message[]) => Message[]
  ) => void
  onDebugUpdate: (
    updater: (prev: PlaygroundDebugData) => PlaygroundDebugData
  ) => void
  onDebugTabChange: (tab: PlaygroundDebugTab) => void
  searchEnabled?: boolean
}

function getLastAssistantMessageKey(messages: Message[]): string {
  for (let index = messages.length - 1; index >= 0; index -= 1) {
    const message = messages[index]
    if (message.from === 'assistant') {
      return message.key
    }
  }

  return ''
}

function updateAssistantMessageByKey(
  messages: Message[],
  messageKey: string,
  updater: (message: Message) => Message
): Message[] {
  if (!messageKey) {
    return updateLastAssistantMessage(messages, updater)
  }

  let updated = false
  const nextMessages = messages.map((message) => {
    if (message.key !== messageKey) return message
    updated = true
    return updater(message)
  })

  return updated ? nextMessages : updateLastAssistantMessage(messages, updater)
}

function isPendingImageAssistantMessage(message: Message | undefined): boolean {
  return (
    message?.from === 'assistant' &&
    (message.status === 'loading' || message.status === 'streaming')
  )
}

function isRecoverableImageWaitMessage(message: Message | undefined): boolean {
  if (!message || message.from !== 'assistant' || message.status === 'error') {
    return false
  }

  return Boolean(
    extractImageGenerationWaitTaskId(message.versions[0]?.content || '')
  )
}

function isActiveImageTaskMessage(message: Message | undefined): boolean {
  return (
    isPendingImageAssistantMessage(message) ||
    isRecoverableImageWaitMessage(message)
  )
}

function readImageErrorMessage(error: unknown, fallback: string): string {
  const err = error as {
    response?: {
      data?: {
        message?: string
        error?: { code?: string; message?: string }
      }
    }
    message?: string
  }

  return (
    err?.response?.data?.error?.message ||
    err?.response?.data?.message ||
    err?.message ||
    fallback
  )
}

function readImageErrorCode(error: unknown): string | undefined {
  const err = error as {
    response?: {
      data?: {
        error?: { code?: string }
      }
    }
  }

  return err?.response?.data?.error?.code || undefined
}

function applyErrorToAssistantMessage(
  message: Message,
  errorMessage: string,
  errorCode?: string
): Message {
  return updateAssistantMessageWithError([message], errorMessage, errorCode)[0]
}

/**
 * Hook for handling chat message sending and receiving
 */
export function useChatHandler({
  config,
  parameterEnabled,
  messages: currentMessages,
  activeSessionId,
  storageUserId,
  onMessageUpdate,
  onSessionMessageUpdate,
  onDebugUpdate,
  onDebugTabChange,
  searchEnabled = false,
}: UseChatHandlerOptions) {
  const { t } = useTranslation()
  const { sendStreamRequest, stopStream, isStreaming } = useStreamRequest()
  const [isImageGenerating, setIsImageGenerating] = useState(false)
  const imageAbortControllerRef = useRef<AbortController | null>(null)
  const currentImageTaskRef = useRef<{
    taskId: string
    messageKey: string
  } | null>(null)
  const currentChatTaskRef = useRef<{
    sessionId: string
    messageKey: string
  } | null>(null)
  const currentMessagesRef = useRef(currentMessages)
  const streamResponseRef = useRef('')

  useEffect(() => {
    currentMessagesRef.current = currentMessages
  }, [currentMessages])

  const commitSessionMessageUpdate = useCallback(
    (sessionId: string, updater: (prev: Message[]) => Message[]) => {
      onSessionMessageUpdate(sessionId, updater)
    },
    [onSessionMessageUpdate]
  )

  const commitActiveMessageUpdate = useCallback(
    (updater: (prev: Message[]) => Message[]) => {
      onMessageUpdate(updater)
    },
    [onMessageUpdate]
  )

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
        buildChatCompletionPayload(messages, config, parameterEnabled, {
          searchEnabled,
        })
      )
    },
    [config, parameterEnabled, searchEnabled]
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
    (
      sessionId: string,
      messageKey: string,
      type: 'reasoning' | 'content',
      chunk: string
    ) => {
      commitSessionMessageUpdate(sessionId, (prev) =>
        updateAssistantMessageByKey(prev, messageKey, (message) => {
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
    [commitSessionMessageUpdate]
  )

  // Handle stream complete
  const handleStreamComplete = useCallback(() => {
    const task = currentChatTaskRef.current
    if (!task) return
    commitSessionMessageUpdate(task.sessionId, (prev) =>
      updateAssistantMessageByKey(prev, task.messageKey, (message) =>
        message.status === MESSAGE_STATUS.COMPLETE ||
        message.status === MESSAGE_STATUS.ERROR
          ? message
          : { ...finalizeMessage(message), status: MESSAGE_STATUS.COMPLETE }
      )
    )
    clearActivePlaygroundChatMessage(task.messageKey, storageUserId)
    currentChatTaskRef.current = null
  }, [commitSessionMessageUpdate, storageUserId])

  // Handle stream error
  const handleStreamError = useCallback(
    (error: string, errorCode?: string) => {
      const task = currentChatTaskRef.current
      toast.error(error)
      if (task) {
        clearActivePlaygroundChatMessage(task.messageKey, storageUserId)
        commitSessionMessageUpdate(task.sessionId, (prev) =>
          updateAssistantMessageByKey(prev, task.messageKey, (message) =>
            applyErrorToAssistantMessage(message, error, errorCode)
          )
        )
        currentChatTaskRef.current = null
        return
      }
      commitActiveMessageUpdate((prev) =>
        updateAssistantMessageWithError(prev, error, errorCode)
      )
    },
    [commitActiveMessageUpdate, commitSessionMessageUpdate, storageUserId]
  )

  const handleImageGenerationError = useCallback(
    (
      sessionId: string,
      messageKey: string,
      error: string,
      errorCode?: string
    ) => {
      toast.error(error)
      commitSessionMessageUpdate(sessionId, (prev) =>
        updateAssistantMessageByKey(prev, messageKey, (message) =>
          applyErrorToAssistantMessage(message, error, errorCode)
        )
      )
    },
    [commitSessionMessageUpdate]
  )

  // Send streaming chat request
  const sendStreamingChat = useCallback(
    (messages: Message[], overridePayload?: PlaygroundRequestPayload) => {
      const payload = getChatPayload(messages, overridePayload)
      const debugId = createPlaygroundDebugId()
      const sessionId = activeSessionId
      const messageKey = getLastAssistantMessageKey(messages)
      if (currentChatTaskRef.current?.messageKey) {
        clearActivePlaygroundChatMessage(
          currentChatTaskRef.current.messageKey,
          storageUserId
        )
      }
      currentChatTaskRef.current = { sessionId, messageKey }
      markActivePlaygroundChatMessage(messageKey, storageUserId)
      startDebugRequest(payload, true)
      sendStreamRequest(
        payload,
        (type, chunk) => handleStreamUpdate(sessionId, messageKey, type, chunk),
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
      activeSessionId,
      getChatPayload,
      startDebugRequest,
      sendStreamRequest,
      handleStreamUpdate,
      completeDebugResponse,
      handleStreamComplete,
      handleStreamError,
      appendDebugSseMessage,
      updateDebugUpstreamRequest,
      storageUserId,
    ]
  )

  // Send non-streaming chat request
  const sendNonStreamingChat = useCallback(
    async (messages: Message[], overridePayload?: PlaygroundRequestPayload) => {
      const payload = getChatPayload(messages, overridePayload)
      const debugId = createPlaygroundDebugId()
      const sessionId = activeSessionId
      const messageKey = getLastAssistantMessageKey(messages)
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

        commitSessionMessageUpdate(sessionId, (prev) =>
          updateAssistantMessageByKey(prev, messageKey, (message) => ({
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
        toast.error(errorMessage)
        commitSessionMessageUpdate(sessionId, (prev) =>
          updateAssistantMessageByKey(prev, messageKey, (message) =>
            applyErrorToAssistantMessage(message, errorMessage, errorCode)
          )
        )
      }
    },
    [
      activeSessionId,
      getChatPayload,
      startDebugRequest,
      completeDebugResponse,
      commitSessionMessageUpdate,
      updateDebugUpstreamRequest,
      fetchAndUpdateDebugUpstreamRequest,
    ]
  )

  const updateImageGenerationMessage = useCallback(
    (
      sessionId: string,
      messageKey: string,
      taskId: string,
      taskData:
        | ImageGenerationTaskResponse
        | ImageGenerationSubmitResponse
        | null,
      attempt: number,
      startedAt: number
    ) => {
      commitSessionMessageUpdate(sessionId, (prev) =>
        updateAssistantMessageByKey(prev, messageKey, (message) => ({
          ...message,
          versions: [
            {
              ...message.versions[0],
              content: getImageGenerationWaitMessage(
                taskId,
                taskData,
                attempt,
                startedAt,
                t
              ),
            },
          ],
          status: MESSAGE_STATUS.LOADING,
        }))
      )
    },
    [commitSessionMessageUpdate, t]
  )

  const completeImageGenerationMessage = useCallback(
    (sessionId: string, messageKey: string, content: string) => {
      commitSessionMessageUpdate(sessionId, (prev) =>
        updateAssistantMessageByKey(prev, messageKey, (message) => ({
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
    [commitSessionMessageUpdate]
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

  const sleepWithImageGenerationProgress = useCallback(
    async (
      ms: number,
      signal: AbortSignal,
      sessionId: string,
      messageKey: string,
      taskId: string,
      taskData:
        | ImageGenerationTaskResponse
        | ImageGenerationSubmitResponse
        | null,
      attempt: number,
      startedAt: number
    ) => {
      const deadline = Date.now() + ms
      let tick = 0

      while (Date.now() < deadline) {
        await sleep(Math.min(1000, Math.max(0, deadline - Date.now())), signal)
        if (signal.aborted || Date.now() >= deadline) {
          continue
        }

        tick += 1
        updateImageGenerationMessage(
          sessionId,
          messageKey,
          taskId,
          taskData,
          attempt + tick,
          startedAt
        )
      }
    },
    [sleep, updateImageGenerationMessage]
  )

  const pollImageGenerationTask = useCallback(
    async (
      taskId: string,
      signal: AbortSignal,
      startedAt: number,
      sessionId: string,
      messageKey: string
    ) => {
      const maxAttempts = 240
      let lastTaskData: ImageGenerationTaskResponse | null = null

      for (let attempt = 0; attempt < maxAttempts; attempt += 1) {
        const taskData = await getImageGenerationTask(taskId, signal)
        lastTaskData = taskData
        const status = String(taskData.status || '').toLowerCase()

        if (!isTerminalImageTaskStatus(status)) {
          updateImageGenerationMessage(
            sessionId,
            messageKey,
            taskId,
            taskData,
            attempt,
            startedAt
          )
          await sleepWithImageGenerationProgress(
            5000,
            signal,
            sessionId,
            messageKey,
            taskId,
            taskData,
            attempt,
            startedAt
          )
          continue
        }

        if (isSuccessfulImageTaskStatus(status)) {
          return taskData
        }

        throw new Error(getImageGenerationFailureMessage(taskData))
      }

      if (lastTaskData) {
        const failureMessage = getImageGenerationFailureMessage(lastTaskData)
        if (failureMessage !== ERROR_MESSAGES.IMAGE_GENERATION_FAILED) {
          throw new Error(failureMessage)
        }
      }

      const lastStatus = String(
        lastTaskData?.raw_status || lastTaskData?.status || ''
      ).trim()
      const suffix = lastStatus ? ` (${lastStatus}, ${taskId})` : ` (${taskId})`
      throw new Error(`${t(ERROR_MESSAGES.IMAGE_GENERATION_TIMEOUT)}${suffix}`)
    },
    [sleepWithImageGenerationProgress, t, updateImageGenerationMessage]
  )

  const isAbortError = useCallback((error: unknown) => {
    const err = error as { name?: string; code?: string; message?: string }
    return (
      err?.name === 'AbortError' ||
      err?.code === 'ERR_CANCELED' ||
      err?.message === 'canceled'
    )
  }, [])

  useEffect(() => {
    let pendingTask = loadPendingImageTasks(storageUserId).find(
      (task) => task.sessionId === activeSessionId
    )
    if (!pendingTask) {
      const recoverableMessage = currentMessagesRef.current.find((message) =>
        isRecoverableImageWaitMessage(message)
      )
      const taskId = extractImageGenerationWaitTaskId(
        recoverableMessage?.versions[0]?.content || ''
      )
      if (recoverableMessage && taskId) {
        pendingTask = {
          taskId,
          messageKey: recoverableMessage.key,
          sessionId: activeSessionId,
          startedAt: Date.now(),
          updatedAt: new Date().toISOString(),
        }
        upsertPendingImageTask(pendingTask, storageUserId)
      }
    }
    if (!pendingTask) return

    const pendingMessage = currentMessagesRef.current.find(
      (message) => message.key === pendingTask.messageKey
    )
    if (!isActiveImageTaskMessage(pendingMessage)) {
      removePendingImageTask(pendingTask.taskId, storageUserId)
      return
    }
    if (currentImageTaskRef.current?.taskId === pendingTask.taskId) {
      return
    }

    imageAbortControllerRef.current?.abort()

    const abortController = new AbortController()
    imageAbortControllerRef.current = abortController
    currentImageTaskRef.current = {
      taskId: pendingTask.taskId,
      messageKey: pendingTask.messageKey,
    }
    setIsImageGenerating(true)
    updateImageGenerationMessage(
      pendingTask.sessionId,
      pendingTask.messageKey,
      pendingTask.taskId,
      null,
      0,
      pendingTask.startedAt
    )

    void (async (task: PendingImageGenerationTask) => {
      try {
        const taskResult = await pollImageGenerationTask(
          task.taskId,
          abortController.signal,
          task.startedAt,
          task.sessionId,
          task.messageKey
        )
        if (task.debugId) {
          await fetchAndUpdateDebugUpstreamRequest(task.debugId)
        }
        if (abortController.signal.aborted) {
          return
        }
        completeDebugResponse(taskResult)
        currentImageTaskRef.current = null
        completeImageGenerationMessage(
          task.sessionId,
          task.messageKey,
          imageTaskResultToMarkdown(task.taskId, taskResult, t)
        )
      } catch (error: unknown) {
        if (isAbortError(error)) {
          return
        }
        if (task.debugId) {
          await fetchAndUpdateDebugUpstreamRequest(task.debugId)
        }
        if (abortController.signal.aborted) {
          return
        }
        currentImageTaskRef.current = null
        const errorMessage = readImageErrorMessage(
          error,
          t(ERROR_MESSAGES.API_REQUEST_ERROR)
        )
        const errorCode = readImageErrorCode(error)
        completeDebugResponse({ error: errorMessage, errorCode })
        handleImageGenerationError(
          task.sessionId,
          task.messageKey,
          errorMessage,
          errorCode
        )
      } finally {
        if (imageAbortControllerRef.current === abortController) {
          imageAbortControllerRef.current = null
        }
        setIsImageGenerating(false)
      }
    })(pendingTask)

    return () => {
      if (imageAbortControllerRef.current === abortController) {
        abortController.abort()
        imageAbortControllerRef.current = null
      }
      if (currentImageTaskRef.current?.taskId === pendingTask.taskId) {
        currentImageTaskRef.current = null
      }
      setIsImageGenerating(false)
    }
  }, [
    activeSessionId,
    completeDebugResponse,
    completeImageGenerationMessage,
    fetchAndUpdateDebugUpstreamRequest,
    handleImageGenerationError,
    isAbortError,
    pollImageGenerationTask,
    storageUserId,
    t,
    updateImageGenerationMessage,
  ])

  useEffect(() => {
    const refreshActiveImageWaitMessages = () => {
      loadPendingImageTasks(storageUserId)
        .filter((task) => task.sessionId === activeSessionId)
        .forEach((task) => {
          const pendingMessage = currentMessagesRef.current.find(
            (message) => message.key === task.messageKey
          )
          if (!isActiveImageTaskMessage(pendingMessage)) {
            return
          }

          updateImageGenerationMessage(
            task.sessionId,
            task.messageKey,
            task.taskId,
            null,
            Math.floor((Date.now() - task.startedAt) / 1000),
            task.startedAt
          )
        })
    }

    refreshActiveImageWaitMessages()
    const intervalId = window.setInterval(refreshActiveImageWaitMessages, 1000)
    return () => window.clearInterval(intervalId)
  }, [activeSessionId, storageUserId, updateImageGenerationMessage])

  useEffect(() => {
    loadPendingImageTasks(storageUserId)
      .filter((task) => task.sessionId === activeSessionId)
      .forEach((task) => {
        const pendingMessage = currentMessages.find(
          (message) => message.key === task.messageKey
        )
        if (!isActiveImageTaskMessage(pendingMessage)) {
          removePendingImageTask(task.taskId, storageUserId)
        }
      })
  }, [activeSessionId, currentMessages, storageUserId])

  const sendImageGenerationChat = useCallback(
    async (messages: Message[], overridePayload?: PlaygroundRequestPayload) => {
      if (currentImageTaskRef.current?.taskId) {
        removePendingImageTask(
          currentImageTaskRef.current.taskId,
          storageUserId
        )
      }
      imageAbortControllerRef.current?.abort()

      const abortController = new AbortController()
      imageAbortControllerRef.current = abortController
      setIsImageGenerating(true)

      const payload = getImagePayload(messages, overridePayload)
      const debugId = createPlaygroundDebugId()
      const startedAt = Date.now()
      const sessionId = activeSessionId
      const messageKey = getLastAssistantMessageKey(messages)
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
          currentImageTaskRef.current = { taskId, messageKey }
          upsertPendingImageTask(
            {
              taskId,
              messageKey,
              sessionId,
              debugId,
              startedAt,
              updatedAt: new Date().toISOString(),
            },
            storageUserId
          )
          updateImageGenerationMessage(
            sessionId,
            messageKey,
            taskId,
            submitData,
            -1,
            startedAt
          )
          const taskResult = await pollImageGenerationTask(
            taskId,
            abortController.signal,
            startedAt,
            sessionId,
            messageKey
          )
          await fetchAndUpdateDebugUpstreamRequest(debugId)
          if (abortController.signal.aborted) {
            return
          }
          completeDebugResponse(taskResult)
          currentImageTaskRef.current = null
          completeImageGenerationMessage(
            sessionId,
            messageKey,
            imageTaskResultToMarkdown(taskId, taskResult, t)
          )
          return
        }

        completeImageGenerationMessage(
          sessionId,
          messageKey,
          imageResponseToMarkdown(submitData) ||
            t(
              'Image generation completed, but the response did not include displayable image data.'
            )
        )
      } catch (error: unknown) {
        if (isAbortError(error)) {
          return
        }
        await fetchAndUpdateDebugUpstreamRequest(debugId)
        if (abortController.signal.aborted) {
          return
        }

        if (currentImageTaskRef.current?.taskId) {
          currentImageTaskRef.current = null
        }
        const errorMessage = readImageErrorMessage(
          error,
          t(ERROR_MESSAGES.API_REQUEST_ERROR)
        )
        const errorCode = readImageErrorCode(error)
        completeDebugResponse({ error: errorMessage, errorCode })
        handleImageGenerationError(
          sessionId,
          messageKey,
          errorMessage,
          errorCode
        )
      } finally {
        if (imageAbortControllerRef.current === abortController) {
          imageAbortControllerRef.current = null
        }
        setIsImageGenerating(false)
      }
    },
    [
      activeSessionId,
      getImagePayload,
      startDebugRequest,
      completeDebugResponse,
      fetchAndUpdateDebugUpstreamRequest,
      updateImageGenerationMessage,
      pollImageGenerationTask,
      completeImageGenerationMessage,
      isAbortError,
      handleImageGenerationError,
      storageUserId,
      t,
    ]
  )

  // Send chat request (stream or non-stream based on config)
  const sendChat = useCallback(
    (messages: Message[], overridePayload?: PlaygroundRequestPayload) => {
      const model = overridePayload
        ? getPayloadModel(overridePayload)
        : config.model
      if (
        isImageGenerationRequestPayload(overridePayload) ||
        shouldUseImageGenerationPayload(messages, model)
      ) {
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
    if (currentImageTaskRef.current?.taskId) {
      removePendingImageTask(currentImageTaskRef.current.taskId, storageUserId)
      currentImageTaskRef.current = null
    }
    imageAbortControllerRef.current?.abort()
    setIsImageGenerating(false)
    stopStream()
    const chatTask = currentChatTaskRef.current
    currentChatTaskRef.current = null
    onDebugUpdate((prev) => ({
      ...prev,
      response:
        prev.isStreaming && prev.response === null
          ? { status: 'stopped', response: streamResponseRef.current.trim() }
          : prev.response,
      isStreaming: false,
    }))
    const updateStoppedMessage = (message: Message) =>
      message.status === MESSAGE_STATUS.LOADING ||
      message.status === MESSAGE_STATUS.STREAMING
        ? { ...finalizeMessage(message), status: MESSAGE_STATUS.COMPLETE }
        : message

    if (chatTask) {
      commitSessionMessageUpdate(chatTask.sessionId, (prev) =>
        updateAssistantMessageByKey(
          prev,
          chatTask.messageKey,
          updateStoppedMessage
        )
      )
      clearActivePlaygroundChatMessage(chatTask.messageKey, storageUserId)
      return
    }

    commitActiveMessageUpdate((prev) =>
      updateLastAssistantMessage(prev, (message) =>
        message.status === MESSAGE_STATUS.LOADING ||
        message.status === MESSAGE_STATUS.STREAMING
          ? { ...finalizeMessage(message), status: MESSAGE_STATUS.COMPLETE }
          : message
      )
    )
  }, [
    stopStream,
    onDebugUpdate,
    commitActiveMessageUpdate,
    commitSessionMessageUpdate,
    storageUserId,
  ])

  return {
    sendChat,
    stopGeneration,
    isGenerating: isStreaming || isImageGenerating,
  }
}
