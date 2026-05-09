import { DEFAULT_CONFIG } from '../constants'
import type {
  ChatCompletionRequest,
  Message,
  PlaygroundConfig,
  ParameterEnabled,
} from '../types'
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
