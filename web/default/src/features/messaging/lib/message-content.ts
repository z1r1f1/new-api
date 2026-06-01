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
export const CHAT_PASTED_IMAGE_MAX_COUNT = 3
export const CHAT_IMAGE_TOTAL_BODY_MAX_LENGTH = 60_000
export const CHAT_IMAGE_MAX_SOURCE_FILE_SIZE = 5 * 1024 * 1024
export const CHAT_IMAGE_MAX_DATA_URL_LENGTH = 54_000
export const CHAT_IMAGE_MAX_DIMENSION = 1280
export const CHAT_IMAGE_OUTPUT_QUALITY = 0.72

const SAFE_IMAGE_DATA_URL_PATTERN =
  /^data:image\/(?:png|jpe?g|gif|webp);base64,[A-Za-z0-9+/=]+$/i
const MESSAGE_IMAGE_PATTERN = /!\[([^\]\n]*)\]\(([^)\s]+)\)/g

export interface ChatImageAttachment {
  id: string
  name: string
  mimeType: string
  size: number
  dataUrl: string
}

export interface ChatImagePreview {
  src: string
  alt: string
}

export type MessageContentPart =
  | { type: 'text'; text: string }
  | { type: 'image'; alt: string; src: string }

export function isSafeChatImageDataUrl(value: string): boolean {
  return SAFE_IMAGE_DATA_URL_PATTERN.test(value)
}

export function isChatMessageSendable(
  text: string,
  attachments: ChatImageAttachment[]
): boolean {
  return Boolean(text.trim() || attachments.length > 0)
}

export function extractMessageContentParts(body: string): MessageContentPart[] {
  const parts: MessageContentPart[] = []
  let cursor = 0

  for (const match of body.matchAll(MESSAGE_IMAGE_PATTERN)) {
    const index = match.index ?? 0
    const fullMatch = match[0]
    const alt = match[1] ?? ''
    const src = match[2] ?? ''

    if (!isSafeChatImageDataUrl(src)) continue

    if (index > cursor) {
      parts.push({ type: 'text', text: body.slice(cursor, index) })
    }
    parts.push({ type: 'image', alt, src })
    cursor = index + fullMatch.length
  }

  if (cursor < body.length) {
    parts.push({ type: 'text', text: body.slice(cursor) })
  }

  if (parts.length === 0) return [{ type: 'text', text: body }]
  return mergeAdjacentTextParts(parts)
}

export function buildChatImageMarkdown(
  attachment: ChatImageAttachment
): string {
  const safeName = attachment.name
    .replace(/[()[\]]/g, ' ')
    .replace(/\s+/g, ' ')
    .replace(/\s+(\.[^.]+)$/, '$1')
    .trim()
  const alt = safeName || 'pasted-image'
  return `![${alt}](${attachment.dataUrl})`
}

export function buildOutgoingMessageBody(
  text: string,
  attachments: ChatImageAttachment[]
): string {
  const bodyParts: string[] = []
  const trimmedText = text.trim()
  if (trimmedText) bodyParts.push(trimmedText)
  for (const attachment of attachments) {
    bodyParts.push(buildChatImageMarkdown(attachment))
  }
  return bodyParts.join('\n\n')
}

export async function createChatImageAttachment(
  file: File,
  id: string
): Promise<ChatImageAttachment> {
  if (!file.type.startsWith('image/')) {
    throw new Error('Only image files can be pasted here')
  }
  if (file.size > CHAT_IMAGE_MAX_SOURCE_FILE_SIZE) {
    throw new Error('Image is too large to send')
  }

  const originalDataUrl = await readFileAsDataUrl(file)
  const shouldKeepOriginal =
    isSafeChatImageDataUrl(originalDataUrl) &&
    originalDataUrl.length <= CHAT_IMAGE_MAX_DATA_URL_LENGTH
  const dataUrl = shouldKeepOriginal
    ? originalDataUrl
    : await resizeImageDataUrl(originalDataUrl)

  if (!isSafeChatImageDataUrl(dataUrl)) {
    throw new Error('Only image files can be pasted here')
  }
  if (dataUrl.length > CHAT_IMAGE_MAX_DATA_URL_LENGTH) {
    throw new Error('Image is too large to send')
  }

  return {
    id,
    name: file.name || 'pasted-image.png',
    mimeType: getMimeTypeFromDataUrl(dataUrl) ?? file.type,
    size: dataUrl.length,
    dataUrl,
  }
}

function mergeAdjacentTextParts(
  parts: MessageContentPart[]
): MessageContentPart[] {
  const merged: MessageContentPart[] = []
  for (const part of parts) {
    const previous = merged.at(-1)
    if (part.type === 'text' && previous?.type === 'text') {
      previous.text += part.text
      continue
    }
    merged.push(part)
  }
  return merged
}

function readFileAsDataUrl(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => {
      if (typeof reader.result === 'string') {
        resolve(reader.result)
        return
      }
      reject(new Error('Failed to read pasted image'))
    }
    reader.onerror = () => reject(new Error('Failed to read pasted image'))
    reader.readAsDataURL(file)
  })
}

function loadImage(dataUrl: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const image = new Image()
    image.onload = () => resolve(image)
    image.onerror = () => reject(new Error('Failed to read pasted image'))
    image.src = dataUrl
  })
}

async function resizeImageDataUrl(dataUrl: string): Promise<string> {
  const image = await loadImage(dataUrl)
  const scale = Math.min(
    1,
    CHAT_IMAGE_MAX_DIMENSION / Math.max(image.naturalWidth, image.naturalHeight)
  )
  const width = Math.max(1, Math.round(image.naturalWidth * scale))
  const height = Math.max(1, Math.round(image.naturalHeight * scale))
  const canvas = document.createElement('canvas')
  canvas.width = width
  canvas.height = height
  const context = canvas.getContext('2d')
  if (!context) throw new Error('Failed to read pasted image')
  context.drawImage(image, 0, 0, width, height)

  const outputType = dataUrl.startsWith('data:image/png')
    ? 'image/png'
    : 'image/jpeg'
  const pngDataUrl = canvas.toDataURL(outputType, CHAT_IMAGE_OUTPUT_QUALITY)
  if (pngDataUrl.length <= CHAT_IMAGE_MAX_DATA_URL_LENGTH) return pngDataUrl
  return canvas.toDataURL('image/jpeg', CHAT_IMAGE_OUTPUT_QUALITY)
}

function getMimeTypeFromDataUrl(dataUrl: string): string | null {
  const match = /^data:([^;,]+)[;,]/.exec(dataUrl)
  return match?.[1] ?? null
}
