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
import { useEffect, useMemo, useRef } from 'react'
import { CheckCheck, Clock3, Copy, Loader2, MessageCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { cn } from '@/lib/utils'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import {
  buildMessageRows,
  formatChatDate,
  formatChatTime,
  getChatUserInitial,
  getMessageReadLabel,
  getMessageSenderName,
} from '../lib/format'
import type { ChatConversation, ChatMessage } from '../types'

interface MessageListProps {
  conversation: ChatConversation | null
  messages: ChatMessage[]
  allMessageCount: number
  currentUserId: number | null
  loading: boolean
  empty: boolean
  searchText: string
  canLoadOlder: boolean
  loadingOlder: boolean
  onLoadOlder: () => void
}

export function MessageList(props: MessageListProps) {
  const { t } = useTranslation()
  const viewportRef = useRef<HTMLDivElement | null>(null)
  const rows = useMemo(() => buildMessageRows(props.messages), [props.messages])

  useEffect(() => {
    const viewport = viewportRef.current
    if (!viewport || props.searchText.trim()) return
    viewport.scrollTo({ top: viewport.scrollHeight, behavior: 'smooth' })
  }, [props.messages.length, props.searchText])

  if (props.empty) {
    return (
      <div className='flex flex-1 items-center justify-center p-6 text-center'>
        <div className='bg-muted/20 max-w-md rounded-3xl border p-8'>
          <div className='bg-primary/10 text-primary mx-auto mb-4 flex size-14 items-center justify-center rounded-2xl'>
            <MessageCircle className='h-6 w-6' />
          </div>
          <h2 className='text-lg font-semibold'>
            {t('Select a conversation')}
          </h2>
          <p className='text-muted-foreground mt-2 text-sm'>
            {t(
              'Choose a user or conversation from the left to start chatting.'
            )}
          </p>
        </div>
      </div>
    )
  }

  if (props.loading) {
    return (
      <div className='text-muted-foreground flex flex-1 items-center justify-center gap-2 p-6 text-sm'>
        <Loader2 className='h-4 w-4 animate-spin' />
        {t('Loading messages...')}
      </div>
    )
  }

  if (props.allMessageCount > 0 && props.messages.length === 0) {
    return (
      <div className='text-muted-foreground flex flex-1 items-center justify-center p-6 text-center text-sm'>
        {t('No messages match this search.')}
      </div>
    )
  }

  if (props.messages.length === 0) {
    return (
      <div className='flex flex-1 items-center justify-center p-6 text-center'>
        <div className='bg-muted/20 max-w-md rounded-3xl border border-dashed p-8'>
          <div className='bg-muted text-muted-foreground mx-auto mb-4 flex size-14 items-center justify-center rounded-2xl'>
            <MessageCircle className='h-6 w-6' />
          </div>
          <h2 className='text-lg font-semibold'>{t('No messages yet')}</h2>
          <p className='text-muted-foreground mt-2 text-sm'>
            {t(
              'Send the first message below to start the conversation timeline.'
            )}
          </p>
        </div>
      </div>
    )
  }

  return (
    <div ref={viewportRef} className='min-h-0 flex-1 overflow-y-auto'>
      <div className='space-y-4 p-4 lg:p-6'>
        {props.canLoadOlder && !props.searchText.trim() && (
          <div className='flex justify-center'>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={props.onLoadOlder}
              disabled={props.loadingOlder}
              className='gap-2 rounded-full'
            >
              {props.loadingOlder ? (
                <Loader2 className='h-4 w-4 animate-spin' />
              ) : (
                <Clock3 className='h-4 w-4' />
              )}
              {t('Load earlier messages')}
            </Button>
          </div>
        )}
        {rows.map((row) => {
          if (row.kind === 'divider') {
            return <DateDivider key={row.key} timestamp={row.timestamp} />
          }
          return (
            <MessageBubble
              key={row.key}
              message={row.message}
              mine={row.message.sender_id === props.currentUserId}
              conversation={props.conversation}
              currentUserId={props.currentUserId}
            />
          )
        })}
      </div>
    </div>
  )
}

interface DateDividerProps {
  timestamp: number
}

function DateDivider(props: DateDividerProps) {
  return (
    <div className='flex items-center gap-3 py-1'>
      <Separator className='flex-1' />
      <Badge
        variant='outline'
        className='bg-background rounded-full px-3 py-1 text-[11px] font-normal'
      >
        {formatChatDate(props.timestamp)}
      </Badge>
      <Separator className='flex-1' />
    </div>
  )
}

interface MessageBubbleProps {
  message: ChatMessage
  mine: boolean
  conversation: ChatConversation | null
  currentUserId: number | null
}

function MessageBubble(props: MessageBubbleProps) {
  const { t } = useTranslation()
  const readLabel = getMessageReadLabel(
    props.message,
    props.conversation,
    props.currentUserId
  )

  const handleCopy = (): void => {
    if (typeof navigator === 'undefined' || !navigator.clipboard) return
    void navigator.clipboard.writeText(props.message.body).then(() => {
      toast.success(t('Message copied'))
    })
  }

  return (
    <div
      className={cn(
        'group flex gap-3',
        props.mine ? 'justify-end' : 'justify-start'
      )}
    >
      {!props.mine && (
        <Avatar size='sm' className='mt-1'>
          <AvatarFallback className='bg-muted text-[10px]'>
            {getChatUserInitial({
              id: props.message.sender_id,
              username: props.message.sender_username,
              display_name: props.message.sender_display_name,
              role: 0,
            })}
          </AvatarFallback>
        </Avatar>
      )}
      <div
        className={cn(
          'flex max-w-[86%] flex-col gap-1',
          props.mine && 'items-end'
        )}
      >
        <div className='text-muted-foreground flex items-center gap-2 px-1 text-[11px]'>
          <span>
            {props.mine ? t('You') : getMessageSenderName(props.message)}
          </span>
          <span>·</span>
          <span>{formatChatTime(props.message.created_at)}</span>
        </div>
        <div
          className={cn(
            'relative rounded-3xl px-4 py-3 text-sm shadow-sm transition-shadow group-hover:shadow-md',
            props.mine
              ? 'bg-primary text-primary-foreground rounded-tr-md'
              : 'bg-card text-card-foreground rounded-tl-md border'
          )}
        >
          <div className='leading-relaxed break-words whitespace-pre-wrap'>
            {props.message.body}
          </div>
        </div>
        <div className='flex items-center gap-1 opacity-0 transition-opacity group-hover:opacity-100'>
          <Button
            type='button'
            variant='ghost'
            size='sm'
            className='h-7 gap-1 px-2 text-xs'
            onClick={handleCopy}
          >
            <Copy className='h-3.5 w-3.5' />
            {t('Copy')}
          </Button>
          {props.mine && (
            <span className='text-muted-foreground flex items-center gap-1 px-1 text-[11px]'>
              <CheckCheck className='h-3.5 w-3.5' />
              {readLabel === 'read' ? t('Read') : t('Unread')}
            </span>
          )}
        </div>
      </div>
    </div>
  )
}
