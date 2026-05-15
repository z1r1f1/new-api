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
import {
  dedupeImageReferenceUrls,
  normalizeImageReferenceUrl,
  parseImageReferencesFromText,
} from './image-references'
import { getCurrentVersion } from './message-utils'

type TranslateFunction = (
  key: string,
  options?: Record<string, unknown>
) => string

const generatedImageMarkdownRegex =
  /!\[([^\]]*)]\((data:image\/[A-Za-z0-9.+-]+;base64,[A-Za-z0-9+/=\r\n]+|\/pg\/(?:public\/)?images\/generations\/[^)\s]+|https?:\/\/[^)\s]+)\)/g

const rawGeneratedImageUrlRegex =
  /(https?:\/\/[^)\s]+\/pg\/(?:public\/)?images\/generations\/[^)\s.,;，。；]+|\/pg\/(?:public\/)?images\/generations\/[^)\s.,;，。；]+)/g

const generatedImageAltRegex = /^image[_\s-]*\d+$|^generated image\s+\d+$/i

const maxReferenceImages = 4

function normalizeGeneratedImageUrl(url: string): string {
  const cleanUrl = normalizeImageReferenceUrl(url)
  if (
    cleanUrl.startsWith('/pg/images/generations/') ||
    cleanUrl.startsWith('/pg/public/images/generations/') ||
    cleanUrl.startsWith('data:image/')
  ) {
    return cleanUrl
  }

  try {
    const parsedUrl = new URL(cleanUrl)
    if (
      parsedUrl.pathname.startsWith('/pg/images/generations/') ||
      parsedUrl.pathname.startsWith('/pg/public/images/generations/')
    ) {
      return cleanUrl
    }
  } catch {
    /* keep the original URL */
  }

  return cleanUrl
}

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

function getLastUserMessageIndex(messages: Message[]): number {
  for (let index = messages.length - 1; index >= 0; index -= 1) {
    if (messages[index]?.from === MESSAGE_ROLES.USER) {
      return index
    }
  }

  return -1
}

function createImageInputField(imageUrls: string[]): string | string[] | null {
  if (imageUrls.length === 0) return null
  if (imageUrls.length === 1) return imageUrls[0]
  return imageUrls
}

function collectPriorImageReferences(
  messages: Message[],
  beforeIndex: number
): string[] {
  const imageUrls = messages
    .slice(0, Math.max(0, beforeIndex))
    .flatMap((message) =>
      parseImageReferencesFromText(getCurrentVersion(message).content).imageUrls
    )

  return dedupeImageReferenceUrls(imageUrls).slice(-maxReferenceImages)
}

export function buildImageGenerationPayload(
  messages: Message[],
  config: PlaygroundConfig
): ImageGenerationRequest {
  const lastUserMessageIndex = getLastUserMessageIndex(messages)
  const lastUserMessage =
    lastUserMessageIndex >= 0 ? messages[lastUserMessageIndex] : null
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

  const parsedPrompt = parseImageReferencesFromText(rawPrompt)
  const prompt = parsedPrompt.text || 'Generate an image'
  const imageInput = createImageInputField(parsedPrompt.imageUrls)
  const referenceImages = collectPriorImageReferences(
    messages,
    lastUserMessageIndex >= 0 ? lastUserMessageIndex : messages.length
  )
  const payload: ImageGenerationRequest = {
    model: config.model,
    group: config.group,
    prompt,
  }

  if (imageInput) {
    payload.image = imageInput
  }
  if (referenceImages.length > 0) {
    payload.reference_images = referenceImages
  }

  return payload
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
  startedAt: number,
  translate?: TranslateFunction
): string {
  const safeAttempt = Math.max(0, attempt)
  const elapsedSeconds = Math.max(
    0,
    Math.floor((Date.now() - startedAt) / 1000)
  )
  const dots = '.'.repeat((safeAttempt % 3) + 1)
  const t = translate ?? ((key: string) => key)
  void taskData

  return [
    `${t('Generating image')}${dots}`,
    '',
    t('This generation may take 2–3 minutes. Please wait patiently.'),
    t('Elapsed: {{elapsed}}', { elapsed: formatElapsed(elapsedSeconds) }),
    '',
    `${t('Task ID:')} \`${taskId}\``,
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
  taskResult: ImageGenerationTaskResponse,
  translate?: TranslateFunction
): string {
  const t = translate ?? ((key: string) => key)
  const items = getImageItems(taskResult)

  const markdown = items
    .map((item, index) => {
      const hasImage = Boolean(item?.url || item?.b64_json)
      if (!hasImage) return ''

      const revisedPrompt = item?.revised_prompt
        ? `**Revised prompt ${index + 1}:** ${item.revised_prompt}\n\n`
        : ''

      const url = item?.url || buildImageTaskContentUrl(taskId, index)
      return `${revisedPrompt}![generated image ${index + 1}](${url})`
    })
    .filter(Boolean)
    .join('\n\n')

  if (markdown) {
    return markdown
  }

  return (
    imageResponseToMarkdown(taskResult) ||
    t(
      'Image generation completed, but the response did not include displayable image data.'
    )
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
          url: normalizeGeneratedImageUrl(url),
        })

        return safeAlt && !generatedImageAltRegex.test(safeAlt)
          ? `\n${safeAlt}\n`
          : '\n'
      }
    )
    .replace(rawGeneratedImageUrlRegex, (_match, url: string) => {
      images.push({
        alt: `generated image ${images.length + 1}`,
        url: normalizeGeneratedImageUrl(url),
      })

      return '\n'
    })
    .replace(/\n{3,}/g, '\n\n')
    .trim()

  return { text, images }
}
