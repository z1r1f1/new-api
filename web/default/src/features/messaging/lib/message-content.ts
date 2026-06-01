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
export const CHAT_STICKER_MAX_COUNT = 6

const SAFE_IMAGE_DATA_URL_PATTERN =
  /^data:image\/(?:png|jpe?g|gif|webp);base64,[A-Za-z0-9+/=]+$/i
const MESSAGE_CONTENT_PATTERN =
  /!\[([^\]\n]*)\]\(([^)\s]+)\)|<!--chat-sticker:([^>]+)-->/g
const MESSAGE_REPLY_PATTERN = /^<!--chat-reply:([^>]+)-->\n*/i
const MESSAGE_MARKER_ID_PATTERN = /^[a-z0-9_-]{1,48}$/i

export interface ChatSticker {
  id: string
  emoji: string
  label: string
  accent: string
}

export interface ChatReactionOption {
  id: string
  emoji: string
  label: string
}

export const AVAILABLE_CHAT_REACTIONS: ChatReactionOption[] = [
  { id: 'thumbs-up', emoji: '👍', label: 'Thumbs up' },
  { id: 'thumbs-down', emoji: '👎', label: 'Thumbs down' },
  { id: 'heart', emoji: '❤️', label: 'Heart' },
  { id: 'laugh', emoji: '😂', label: 'Laugh' },
  { id: 'surprised', emoji: '😮', label: 'Surprised' },
  { id: 'sad', emoji: '😢', label: 'Sad' },
  { id: 'angry', emoji: '😡', label: 'Angry' },
  { id: 'party', emoji: '🎉', label: 'Party' },
  { id: 'fire', emoji: '🔥', label: 'Fire' },
  { id: 'rocket', emoji: '🚀', label: 'Rocket' },
  { id: 'clap', emoji: '👏', label: 'Clap' },
  { id: 'hands', emoji: '🙌', label: 'Hands' },
  { id: 'pray', emoji: '🙏', label: 'Pray' },
  { id: 'handshake', emoji: '🤝', label: 'Handshake' },
  { id: 'hundred', emoji: '💯', label: 'Hundred' },
  { id: 'sparkles', emoji: '✨', label: 'Sparkles' },
  { id: 'check', emoji: '✅', label: 'Check' },
  { id: 'cross', emoji: '❌', label: 'Cross' },
  { id: 'eyes', emoji: '👀', label: 'Eyes' },
  { id: 'thinking', emoji: '🤔', label: 'Thinking' },
  { id: 'cool', emoji: '😎', label: 'Cool' },
  { id: 'smiling-hearts', emoji: '🥰', label: 'Smiling hearts' },
  { id: 'heart-eyes', emoji: '😍', label: 'Heart eyes' },
  { id: 'star-struck', emoji: '🤩', label: 'Star struck' },
  { id: 'sweat-smile', emoji: '😅', label: 'Sweat smile' },
  { id: 'rolling-laugh', emoji: '🤣', label: 'Rolling laugh' },
  { id: 'crying', emoji: '😭', label: 'Crying' },
  { id: 'triumph', emoji: '😤', label: 'Triumph' },
  { id: 'scream', emoji: '😱', label: 'Scream' },
  { id: 'salute', emoji: '🫡', label: 'Salute' },
  { id: 'muscle', emoji: '💪', label: 'Muscle' },
  { id: 'brain', emoji: '🧠', label: 'Brain' },
  { id: 'idea', emoji: '💡', label: 'Idea' },
  { id: 'star', emoji: '⭐', label: 'Star' },
  { id: 'glowing-star', emoji: '🌟', label: 'Glowing star' },
  { id: 'trophy', emoji: '🏆', label: 'Trophy' },
  { id: 'pin', emoji: '📌', label: 'Pin' },
  { id: 'memo', emoji: '📝', label: 'Memo' },
  { id: 'cheers', emoji: '🍻', label: 'Cheers' },
  { id: 'coffee', emoji: '☕', label: 'Coffee' },
  { id: 'dog', emoji: '🐶', label: 'Dog' },
  { id: 'cat', emoji: '🐱', label: 'Cat' },
  { id: 'panda', emoji: '🐼', label: 'Panda' },
  { id: 'penguin', emoji: '🐧', label: 'Penguin' },
  { id: 'rainbow', emoji: '🌈', label: 'Rainbow' },
  { id: 'zap', emoji: '⚡', label: 'Zap' },
  { id: 'gem', emoji: '💎', label: 'Gem' },
  { id: 'target', emoji: '🎯', label: 'Target' },
]

export const AVAILABLE_CHAT_STICKERS: ChatSticker[] =
  AVAILABLE_CHAT_REACTIONS.map((reaction) => ({
    ...reaction,
    accent: '',
  }))

const DEFAULT_CHAT_STICKER: ChatSticker = {
  id: 'custom',
  emoji: '🙂',
  label: 'Sticker',
  accent: 'from-muted via-muted to-muted',
}

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

export interface ChatReplyReference {
  messageId: number
  senderName: string
  preview: string
}

export type MessageContentPart =
  | { type: 'text'; text: string }
  | { type: 'image'; alt: string; src: string }
  | { type: 'sticker'; sticker: ChatSticker }

export function isSafeChatImageDataUrl(value: string): boolean {
  return SAFE_IMAGE_DATA_URL_PATTERN.test(value)
}

export function isChatMessageSendable(
  text: string,
  attachments: ChatImageAttachment[],
  stickers: ChatSticker[] = []
): boolean {
  return Boolean(text.trim() || attachments.length > 0 || stickers.length > 0)
}

export function extractMessageReplyReference(
  body: string
): ChatReplyReference | null {
  const match = MESSAGE_REPLY_PATTERN.exec(body)
  if (!match) return null
  return parseReplyReferencePayload(match[1] ?? '')
}

export function extractMessageContentParts(body: string): MessageContentPart[] {
  const contentBody = stripMessageReplyReference(body)
  const parts: MessageContentPart[] = []
  let cursor = 0

  for (const match of contentBody.matchAll(MESSAGE_CONTENT_PATTERN)) {
    const index = match.index ?? 0
    const fullMatch = match[0]
    const alt = match[1] ?? ''
    const src = match[2] ?? ''
    const stickerPayload = match[3] ?? ''

    const part = stickerPayload
      ? getStickerContentPart(stickerPayload)
      : getImageContentPart(alt, src)
    if (!part) continue

    if (index > cursor)
      parts.push({ type: 'text', text: contentBody.slice(cursor, index) })
    parts.push(part)
    cursor = index + fullMatch.length
  }

  if (cursor < contentBody.length) {
    parts.push({ type: 'text', text: contentBody.slice(cursor) })
  }

  if (parts.length === 0) return [{ type: 'text', text: contentBody }]
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

export function buildChatStickerMessage(
  sticker: ChatSticker,
  replyReference?: ChatReplyReference | null
): string {
  return buildOutgoingMessageBody('', [], replyReference, [sticker])
}

export function buildOutgoingMessageBody(
  text: string,
  attachments: ChatImageAttachment[],
  replyReference?: ChatReplyReference | null,
  stickers: ChatSticker[] = []
): string {
  const bodyParts: string[] = []
  if (replyReference) bodyParts.push(buildChatReplyMarker(replyReference))
  const trimmedText = text.trim()
  if (trimmedText) bodyParts.push(trimmedText)
  for (const attachment of attachments) {
    bodyParts.push(buildChatImageMarkdown(attachment))
  }
  for (const sticker of stickers) {
    bodyParts.push(buildChatStickerMarker(sticker))
  }
  return bodyParts.join('\n\n')
}

export function getMessageReplyPreview(body: string): string {
  const parts = extractMessageContentParts(body)
  for (const part of parts) {
    if (part.type === 'text') {
      const text = part.text.trim().replace(/\s+/g, ' ')
      if (text) return truncateMessagePreview(text)
      continue
    }
    if (part.type === 'image') return 'Image'
    return part.sticker.emoji
  }
  return ''
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

function stripMessageReplyReference(body: string): string {
  return body.replace(MESSAGE_REPLY_PATTERN, '')
}

function buildChatReplyMarker(reference: ChatReplyReference): string {
  const payload = {
    messageId: Math.max(0, Math.trunc(reference.messageId)),
    senderName: truncateMessagePreview(reference.senderName.trim() || 'User'),
    preview: truncateMessagePreview(reference.preview.trim()),
  }
  return `<!--chat-reply:${encodeMarkerPayload(payload)}-->`
}

function buildChatStickerMarker(sticker: ChatSticker): string {
  const payload = {
    id: sticker.id,
    emoji: sticker.emoji,
    label: sticker.label,
  }
  return `<!--chat-sticker:${encodeMarkerPayload(payload)}-->`
}

function getImageContentPart(
  alt: string,
  src: string
): MessageContentPart | null {
  if (!isSafeChatImageDataUrl(src)) return null
  return { type: 'image', alt, src }
}

function getStickerContentPart(payload: string): MessageContentPart | null {
  const sticker = parseStickerPayload(payload)
  if (!sticker) return null
  return { type: 'sticker', sticker }
}

function parseReplyReferencePayload(
  payload: string
): ChatReplyReference | null {
  const value = decodeMarkerPayload(payload)
  if (!isRecord(value)) return null
  const messageId = Number(value.messageId)
  const senderName = getMarkerString(value.senderName, 80)
  const preview = getMarkerString(value.preview, 160)
  if (!Number.isFinite(messageId) || messageId < 0) return null
  if (!senderName) return null
  return {
    messageId: Math.trunc(messageId),
    senderName,
    preview,
  }
}

function parseStickerPayload(payload: string): ChatSticker | null {
  const value = decodeMarkerPayload(payload)
  if (!isRecord(value)) return null
  const id = getMarkerString(value.id, 48)
  const emoji = getMarkerString(value.emoji, 16)
  const label = getMarkerString(value.label, 40)
  if (!MESSAGE_MARKER_ID_PATTERN.test(id) || !emoji || !label) return null
  const knownSticker = AVAILABLE_CHAT_STICKERS.find(
    (sticker) => sticker.id === id
  )
  return {
    id,
    emoji,
    label,
    accent: knownSticker?.accent ?? DEFAULT_CHAT_STICKER.accent,
  }
}

function encodeMarkerPayload(value: object): string {
  return encodeURIComponent(JSON.stringify(value))
}

function decodeMarkerPayload(payload: string): unknown {
  try {
    return JSON.parse(decodeURIComponent(payload))
  } catch {
    return null
  }
}

function getMarkerString(value: unknown, maxLength: number): string {
  if (typeof value !== 'string') return ''
  return value.trim().slice(0, maxLength)
}

function truncateMessagePreview(value: string): string {
  const normalized = value.trim().replace(/\s+/g, ' ')
  if (normalized.length <= 120) return normalized
  return `${normalized.slice(0, 119)}…`
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
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
