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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import {
  extractMessageContentParts,
  extractMessageReplyReference,
  type ChatImagePreview,
  type ChatSticker,
} from '../lib/message-content'
import { ChatImagePreviewDialog } from './chat-image-preview-dialog'

interface MessageContentProps {
  body: string
  mine: boolean
  onReplyReferenceClick?: (messageId: number) => void
}

export function MessageContent(props: MessageContentProps) {
  const { t } = useTranslation()
  const [previewImage, setPreviewImage] = useState<ChatImagePreview | null>(
    null
  )
  const replyReference = extractMessageReplyReference(props.body)
  const parts = extractMessageContentParts(props.body)

  return (
    <>
      <div className='flex flex-col gap-2'>
        {replyReference && (
          <ReplyReferenceBlock
            mine={props.mine}
            senderName={replyReference.senderName}
            preview={replyReference.preview || t('Message')}
            onClick={
              props.onReplyReferenceClick
                ? () => props.onReplyReferenceClick?.(replyReference.messageId)
                : undefined
            }
          />
        )}
        {parts.map((part, index) => {
          if (part.type === 'text') {
            if (!part.text) return null
            return (
              <span key={`text-${index}`} className='whitespace-pre-wrap'>
                {part.text}
              </span>
            )
          }
          if (part.type === 'sticker') {
            return (
              <ChatStickerContent
                key={`sticker-${index}`}
                sticker={part.sticker}
              />
            )
          }
          const alt = part.alt || t('Image Preview')
          return (
            <button
              key={`image-${index}`}
              type='button'
              onClick={() => setPreviewImage({ src: part.src, alt })}
              className='focus-visible:ring-ring block max-w-full cursor-zoom-in rounded-2xl text-left transition-transform outline-none hover:scale-[1.01] focus-visible:ring-2 focus-visible:ring-offset-2'
              aria-label={t('Image Preview')}
            >
              <img
                src={part.src}
                alt={alt}
                loading='lazy'
                className={cn(
                  'max-h-80 max-w-full rounded-2xl border object-contain shadow-sm',
                  props.mine ? 'border-primary-foreground/25' : 'border-border'
                )}
              />
            </button>
          )
        })}
      </div>
      <ChatImagePreviewDialog
        image={previewImage}
        open={Boolean(previewImage)}
        onOpenChange={(open) => {
          if (!open) setPreviewImage(null)
        }}
      />
    </>
  )
}

interface ChatStickerContentProps {
  sticker: ChatSticker
}

function ChatStickerContent(props: ChatStickerContentProps) {
  const { t } = useTranslation()

  return (
    <div
      className={cn(
        'inline-flex w-fit items-center gap-1.5 rounded-2xl border bg-gradient-to-br px-3 py-2 shadow-sm',
        props.sticker.accent
      )}
    >
      <span className='text-3xl leading-none'>{props.sticker.emoji}</span>
      <span className='text-[11px] font-medium text-slate-700'>
        {t(props.sticker.label)}
      </span>
    </div>
  )
}

interface ReplyReferenceBlockProps {
  mine: boolean
  senderName: string
  preview: string
  onClick?: () => void
}

function ReplyReferenceBlock(props: ReplyReferenceBlockProps) {
  const { t } = useTranslation()
  const className = cn(
    'mb-1 w-full rounded-2xl border-l-4 px-3 py-2 text-left text-xs transition-colors',
    props.mine
      ? 'border-green-700/40 bg-white/35 text-slate-900/85'
      : 'border-primary/50 bg-muted/60 text-muted-foreground',
    props.onClick &&
      (props.mine
        ? 'cursor-pointer hover:bg-white/45 focus-visible:ring-green-700/40'
        : 'cursor-pointer hover:bg-muted focus-visible:ring-ring'),
    props.onClick && 'outline-none focus-visible:ring-2 focus-visible:ring-offset-2'
  )

  const content = (
    <>
      <div className='font-medium'>
        {t('Replying to {{name}}', {
          name: props.senderName,
        })}
      </div>
      <div className='mt-0.5 line-clamp-2'>{props.preview}</div>
    </>
  )

  if (!props.onClick) {
    return <div className={className}>{content}</div>
  }

  return (
    <button
      type='button'
      className={className}
      onClick={props.onClick}
      aria-label={t('Jump to replied message')}
    >
      {content}
    </button>
  )
}
