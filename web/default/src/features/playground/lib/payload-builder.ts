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
import { DEFAULT_CONFIG } from '../constants'
import type {
  ChatCompletionRequest,
  Message,
  PlaygroundConfig,
  ParameterEnabled,
  PlaygroundRequestPayload,
} from '../types'
import { buildImageGenerationPayload, isImageGenerationModel } from './image-generation'
import { formatMessageForAPI, isValidMessage } from './message-utils'

const omitWhenDefaultParameterKeys: Array<keyof ParameterEnabled> = [
  'temperature',
  'top_p',
  'frequency_penalty',
  'presence_penalty',
]

function shouldIncludeParameter(
  key: keyof ParameterEnabled,
  config: PlaygroundConfig,
  parameterEnabled: ParameterEnabled
) {
  if (!parameterEnabled[key]) {
    return false
  }

  const value = config[key as keyof PlaygroundConfig]
  if (value === undefined || value === null) {
    return false
  }

  if (
    omitWhenDefaultParameterKeys.includes(key) &&
    value === DEFAULT_CONFIG[key as keyof PlaygroundConfig]
  ) {
    return false
  }

  return true
}

/**
 * Build API request payload from messages and config
 */
export function buildChatCompletionPayload(
  messages: Message[],
  config: PlaygroundConfig,
  parameterEnabled: ParameterEnabled
): ChatCompletionRequest {
  // Filter and format valid messages
  const processedMessages = messages
    .filter(isValidMessage)
    .map(formatMessageForAPI)

  const payload: ChatCompletionRequest = {
    model: config.model,
    group: config.group,
    messages: processedMessages,
    stream: config.stream,
  }

  // Add enabled parameters
  const parameterKeys: Array<keyof ParameterEnabled> = [
    'temperature',
    'top_p',
    'max_tokens',
    'frequency_penalty',
    'presence_penalty',
    'seed',
  ]

  parameterKeys.forEach((key) => {
    if (shouldIncludeParameter(key, config, parameterEnabled)) {
      const value = config[key as keyof PlaygroundConfig]
      ;(payload as unknown as Record<string, unknown>)[key] = value
    }
  })

  return payload
}

export function parseCustomRequestBody(
  customRequestBody: string
): { payload: PlaygroundRequestPayload | null; error: string | null } {
  const trimmed = customRequestBody.trim()
  if (!trimmed) {
    return { payload: null, error: null }
  }

  try {
    const parsed = JSON.parse(trimmed) as unknown
    if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) {
      return { payload: null, error: 'Custom request body must be a JSON object' }
    }
    return { payload: parsed as PlaygroundRequestPayload, error: null }
  } catch (error) {
    return {
      payload: null,
      error: error instanceof Error ? error.message : String(error),
    }
  }
}

export function buildPlaygroundPreviewPayload(params: {
  messages: Message[]
  config: PlaygroundConfig
  parameterEnabled: ParameterEnabled
  customRequestMode: boolean
  customRequestBody: string
}): { payload: PlaygroundRequestPayload | null; error: string | null } {
  if (params.customRequestMode) {
    return parseCustomRequestBody(params.customRequestBody)
  }

  if (isImageGenerationModel(params.config.model)) {
    return {
      payload: buildImageGenerationPayload(params.messages, params.config),
      error: null,
    }
  }

  return {
    payload: buildChatCompletionPayload(
      params.messages,
      params.config,
      params.parameterEnabled
    ),
    error: null,
  }
}
