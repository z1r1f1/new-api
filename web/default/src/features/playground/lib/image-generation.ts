import { API_ENDPOINTS, ERROR_MESSAGES, MESSAGE_ROLES } from '../constants'
import type {
  GeneratedImage,
  ImageGenerationRequest,
  ImageGenerationResult,
  ImageGenerationSubmitResponse,
  ImageGenerationTaskResponse,
  Message,
  PlaygroundConfig,
} from '../types'
import { getCurrentVersion } from './message-utils'

const generatedImageMarkdownRegex =
  /!\[([^\]]*)]\((data:image\/[A-Za-z0-9.+-]+;base64,[A-Za-z0-9+/=\r\n]+|\/pg\/images\/generations\/[^)\s]+|https?:\/\/[^)\s]+)\)/g

const generatedImageAltRegex = /^image[_\s-]*\d+$|^generated image\s+\d+$/i

export function isImageGenerationModel(model: string): boolean {
  const normalized = String(model || '')
    .trim()
    .toLowerCase()

  return (
    normalized.startsWith('gpt-image') ||
    normalized.startsWith('chatgpt-image') ||
    normalized.startsWith('dall-e')
  )
}

function parseJsonObject(value: string): Record<string, unknown> | null {
  const candidates = [value]
  const firstBrace = value.indexOf('{')
  const lastBrace = value.lastIndexOf('}')
  if (firstBrace >= 0 && lastBrace > firstBrace) {
    candidates.push(value.slice(firstBrace, lastBrace + 1))
  }

  for (const candidate of candidates) {
    const trimmed = candidate.trim()
    if (!trimmed) continue

    try {
      const parsed: unknown = JSON.parse(trimmed)
      if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
        return parsed as Record<string, unknown>
      }
    } catch {
      /* try next candidate */
    }
  }

  return null
}

function isJsonImagePayload(
  payload: Record<string, unknown> | null
): payload is Record<string, unknown> {
  return !!payload && Object.prototype.hasOwnProperty.call(payload, 'prompt')
}

function createPrompt(value: unknown, fallback: string): string {
  return String(value || '').trim() || fallback
}

function sanitizeJsonImagePayload(
  payload: Record<string, unknown>
): Record<string, unknown> {
  const sanitizedPayload = { ...payload }
  delete sanitizedPayload.messages
  delete sanitizedPayload.stream
  return sanitizedPayload
}

function getLastUserMessage(messages: Message[]): Message | null {
  for (let index = messages.length - 1; index >= 0; index -= 1) {
    const message = messages[index]
    if (message?.from === MESSAGE_ROLES.USER) {
      return message
    }
  }

  return null
}

export function buildImageGenerationPayload(
  messages: Message[],
  config: PlaygroundConfig
): ImageGenerationRequest {
  const lastUserMessage = getLastUserMessage(messages)
  const rawPrompt = lastUserMessage
    ? getCurrentVersion(lastUserMessage).content.trim()
    : ''
  const jsonPayload = parseJsonObject(rawPrompt)

  if (isJsonImagePayload(jsonPayload)) {
    const sanitizedPayload = sanitizeJsonImagePayload(jsonPayload)
    return {
      ...sanitizedPayload,
      model: config.model,
      group: config.group,
      prompt: createPrompt(sanitizedPayload.prompt, 'Generate an image'),
    }
  }

  return {
    model: config.model,
    group: config.group,
    prompt: rawPrompt || 'Generate an image',
  }
}

export function extractImageTaskId(
  response: ImageGenerationSubmitResponse | ImageGenerationTaskResponse
): string {
  return String(response.task_id || response.taskId || response.id || '').trim()
}

export function isTerminalImageTaskStatus(status?: string): boolean {
  const normalized = String(status || '').toLowerCase()
  return ['succeeded', 'failed', 'success', 'failure', 'completed'].includes(
    normalized
  )
}

export function isSuccessfulImageTaskStatus(status?: string): boolean {
  const normalized = String(status || '').toLowerCase()
  return ['succeeded', 'success', 'completed'].includes(normalized)
}

function formatElapsed(seconds: number): string {
  if (seconds < 60) {
    return `${seconds}s`
  }

  const minutes = Math.floor(seconds / 60)
  const remainingSeconds = seconds % 60
  return remainingSeconds > 0
    ? `${minutes}m ${remainingSeconds}s`
    : `${minutes}m`
}

export function getImageGenerationWaitMessage(
  taskId: string,
  taskData: ImageGenerationTaskResponse | ImageGenerationSubmitResponse | null,
  attempt: number,
  startedAt: number
): string {
  const safeAttempt = Math.max(0, attempt)
  const elapsedSeconds = Math.max(
    0,
    Math.floor((Date.now() - startedAt) / 1000)
  )
  const dots = '.'.repeat((safeAttempt % 3) + 1)
  const progress = String(taskData?.progress || '').trim()
  const progressLine =
    progress && progress !== '1%' ? `Progress: ${progress}` : ''

  let stage = 'Task submitted, waiting for image generation to start'
  if (elapsedSeconds >= 20) {
    stage = 'Image generation is running and may take 1-3 minutes'
  }
  if (elapsedSeconds >= 90) {
    stage = 'Still waiting for the upstream image result'
  }
  if (elapsedSeconds >= 240) {
    stage = 'Continuing to wait for the upstream task to finish'
  }

  return [
    `Generating image${dots}`,
    '',
    `Status: ${stage}`,
    `Elapsed: ${formatElapsed(elapsedSeconds)}`,
    progressLine,
    '',
    `Task ID: \`${taskId}\``,
  ]
    .filter(Boolean)
    .join('\n')
}

export function buildImageTaskContentUrl(
  taskId: string,
  index: number
): string {
  return `${API_ENDPOINTS.IMAGE_GENERATIONS}/${encodeURIComponent(taskId)}/image/${index}`
}

function getImageItems(
  response: ImageGenerationResult | ImageGenerationTaskResponse
) {
  const resultData =
    'data' in response && !Array.isArray(response.data) && response.data
      ? response.data
      : response

  return Array.isArray(resultData?.data) ? resultData.data : []
}

export function imageResponseToMarkdown(
  response: ImageGenerationResult | ImageGenerationTaskResponse
): string {
  const items = getImageItems(response)

  const markdown = items
    .map((item, index) => {
      const url =
        item?.url ||
        (item?.b64_json ? `data:image/png;base64,${item.b64_json}` : '')
      if (!url) return ''

      const revisedPrompt = item?.revised_prompt
        ? `**Revised prompt ${index + 1}:** ${item.revised_prompt}\n\n`
        : ''

      return `${revisedPrompt}![generated image ${index + 1}](${url})`
    })
    .filter(Boolean)
    .join('\n\n')

  return markdown
}

export function imageTaskResultToMarkdown(
  taskId: string,
  taskResult: ImageGenerationTaskResponse
): string {
  const items = getImageItems(taskResult)

  const markdown = items
    .map((item, index) => {
      const hasImage = Boolean(item?.url || item?.b64_json)
      if (!hasImage) return ''

      const revisedPrompt = item?.revised_prompt
        ? `**Revised prompt ${index + 1}:** ${item.revised_prompt}\n\n`
        : ''

      return `${revisedPrompt}![generated image ${index + 1}](${buildImageTaskContentUrl(taskId, index)})`
    })
    .filter(Boolean)
    .join('\n\n')

  if (markdown) {
    return markdown
  }

  return (
    imageResponseToMarkdown(taskResult) ||
    'Image generation completed, but the response did not include displayable image data.'
  )
}

export function getImageGenerationFailureMessage(
  taskData: ImageGenerationTaskResponse
): string {
  const error = taskData.data?.error?.message || taskData.data?.message
  return taskData.fail_reason || error || ERROR_MESSAGES.IMAGE_GENERATION_FAILED
}

export function parseGeneratedImagesFromMarkdown(content: string): {
  text: string
  images: GeneratedImage[]
} {
  if (!content.trim()) {
    return { text: '', images: [] }
  }

  const images: GeneratedImage[] = []
  const text = content
    .replace(
      generatedImageMarkdownRegex,
      (_match, alt: string, url: string) => {
        const safeAlt = String(alt || '').trim()
        images.push({
          alt: safeAlt || `generated image ${images.length + 1}`,
          url: String(url || '').replace(/[\r\n]/g, ''),
        })

        return safeAlt && !generatedImageAltRegex.test(safeAlt)
          ? `\n${safeAlt}\n`
          : '\n'
      }
    )
    .replace(/\n{3,}/g, '\n\n')
    .trim()

  return { text, images }
}
