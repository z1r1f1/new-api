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
import { useMemo, type ClipboardEvent } from 'react'
import { AtSign, ImagePlus, Loader2, Send, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { getChatUserDisplayName } from '../lib/format'
import type { ChatImageAttachment } from '../lib/message-content'
import type { ChatUser } from '../types'

interface MessageComposerProps {
  value: string
  active: boolean
  sending: boolean
  processingImages: boolean
  maxLength: number
  mentionUsers: ChatUser[]
  attachments: ChatImageAttachment[]
  onChange: (value: string) => void
  onSend: () => void
  onPasteImages: (files: File[]) => void
  onRemoveAttachment: (id: string) => void
}

export function MessageComposer(props: MessageComposerProps) {
  const { t } = useTranslation()
  const remaining = props.maxLength - props.value.length
  const mentionQuery = getTrailingMentionQuery(props.value)
  const mentionCandidates = useMemo(
    () => filterMentionCandidates(props.mentionUsers, mentionQuery),
    [mentionQuery, props.mentionUsers]
  )
  const showMentionCandidates = Boolean(
    props.active && !props.sending && mentionQuery !== null
  )
  const hasContent = Boolean(props.value.trim() || props.attachments.length > 0)

  const handleMention = (user: ChatUser): void => {
    const mention = `@${user.username} `
    if (mentionQuery === null) {
      props.onChange(props.value ? `${props.value}${mention}` : mention)
      return
    }
    const atIndex = props.value.lastIndexOf('@')
    if (atIndex < 0) {
      props.onChange(props.value ? `${props.value}${mention}` : mention)
      return
    }
    props.onChange(`${props.value.slice(0, atIndex)}${mention}`)
  }

  const handlePaste = (event: ClipboardEvent<HTMLTextAreaElement>): void => {
    const files = getImageFilesFromClipboard(event)
    if (files.length === 0) return
    event.preventDefault()
    props.onPasteImages(files)
  }

  return (
    <div className='border-border/80 bg-background shrink-0 border-t p-3 lg:p-4'>
      <div className='focus-within:border-primary/50 relative rounded-2xl border p-2'>
        {showMentionCandidates && mentionCandidates.length > 0 && (
          <div className='bg-popover text-popover-foreground absolute right-2 bottom-[calc(100%-0.25rem)] left-2 z-10 rounded-xl border p-2 shadow-lg'>
            <div className='text-muted-foreground mb-1 flex items-center gap-1 px-1 text-xs'>
              <AtSign className='h-3.5 w-3.5' />
              {t('Mention')}
            </div>
            <div className='max-h-48 overflow-y-auto'>
              {mentionCandidates.map((user) => (
                <button
                  key={user.id}
                  type='button'
                  className='hover:bg-muted flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm'
                  onClick={() => handleMention(user)}
                >
                  <span className='font-medium'>@{user.username}</span>
                  {getChatUserDisplayName(user) !== user.username && (
                    <span className='text-muted-foreground truncate text-xs'>
                      {getChatUserDisplayName(user)}
                    </span>
                  )}
                </button>
              ))}
            </div>
          </div>
        )}
        {props.attachments.length > 0 && (
          <div className='mb-2 flex flex-wrap gap-2 border-b pb-2'>
            {props.attachments.map((attachment) => (
              <div
                key={attachment.id}
                className='group/attachment bg-muted relative h-20 w-20 overflow-hidden rounded-xl border'
              >
                <img
                  src={attachment.dataUrl}
                  alt={attachment.name}
                  className='size-full object-cover'
                />
                <button
                  type='button'
                  className='bg-background/90 text-foreground absolute top-1 right-1 flex size-6 items-center justify-center rounded-full opacity-0 shadow transition-opacity group-hover/attachment:opacity-100 focus:opacity-100'
                  onClick={() => props.onRemoveAttachment(attachment.id)}
                  aria-label={t('Remove image')}
                >
                  <X className='h-3.5 w-3.5' />
                </button>
              </div>
            ))}
          </div>
        )}
        <Textarea
          value={props.value}
          onChange={(event) => props.onChange(event.target.value)}
          onPaste={handlePaste}
          placeholder={t('Paste images or type a message...')}
          disabled={!props.active || props.sending || props.processingImages}
          className='min-h-20 resize-none border-0 bg-transparent shadow-none focus-visible:ring-0'
          onKeyDown={(event) => {
            if (event.key === 'Enter' && !event.shiftKey) {
              event.preventDefault()
              props.onSend()
            }
          }}
        />
        <div className='flex items-center justify-between gap-3 border-t pt-2'>
          <div className='flex min-w-0 flex-wrap items-center gap-2'>
            <span
              className={cn(
                'text-muted-foreground px-1 text-xs tabular-nums',
                remaining < 0 && 'text-destructive'
              )}
            >
              {t('{{count}} characters left', { count: remaining })}
            </span>
            {props.processingImages && (
              <span className='text-muted-foreground flex items-center gap-1 text-xs'>
                <Loader2 className='h-3.5 w-3.5 animate-spin' />
                {t('Processing image...')}
              </span>
            )}
            {props.attachments.length > 0 && !props.processingImages && (
              <span className='text-muted-foreground flex items-center gap-1 text-xs'>
                <ImagePlus className='h-3.5 w-3.5' />
                {t('{{count}} images attached', {
                  count: props.attachments.length,
                })}
              </span>
            )}
          </div>
          <Button
            type='button'
            onClick={props.onSend}
            disabled={
              !props.active ||
              props.sending ||
              props.processingImages ||
              !hasContent
            }
            className='gap-2'
          >
            {props.sending ? (
              <Loader2 className='h-4 w-4 animate-spin' />
            ) : (
              <Send className='h-4 w-4' />
            )}
            {t('Send')}
          </Button>
        </div>
      </div>
    </div>
  )
}

function getTrailingMentionQuery(value: string): string | null {
  const match = /(^|\s)@([^\s@]*)$/.exec(value)
  return match?.[2] ?? null
}

function filterMentionCandidates(
  users: ChatUser[],
  query: string | null
): ChatUser[] {
  if (query === null) return []
  const keyword = query.trim().toLowerCase()
  return users
    .filter((user) => {
      if (!keyword) return true
      return (
        user.username.toLowerCase().includes(keyword) ||
        getChatUserDisplayName(user).toLowerCase().includes(keyword)
      )
    })
    .slice(0, 8)
}

function getImageFilesFromClipboard(
  event: ClipboardEvent<HTMLTextAreaElement>
): File[] {
  const files: File[] = []
  for (const item of Array.from(event.clipboardData.items)) {
    if (item.kind !== 'file') continue
    const file = item.getAsFile()
    if (!file?.type.startsWith('image/')) continue
    files.push(file)
  }
  return files
}
