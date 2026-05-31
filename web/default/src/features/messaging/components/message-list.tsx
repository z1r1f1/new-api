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
import {
  Check,
  CheckCheck,
  Clock3,
  Copy,
  Eye,
  EyeOff,
  Loader2,
  MessageCircle,
  Undo2,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { cn } from '@/lib/utils'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTitle,
  PopoverTrigger,
} from '@/components/ui/popover'
import { Separator } from '@/components/ui/separator'
import {
  buildMessageRows,
  formatChatDate,
  formatChatTime,
  getChatUserInitial,
  getChatUserDisplayName,
  getMessageReadLabel,
  getMessageReceiptSummary,
  getMessageSenderName,
} from '../lib/format'
import type { ChatConversation, ChatMessage, ChatUser } from '../types'

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
  recallingMessageId: number | null
  onLoadOlder: () => void
  onRecallMessage: (message: ChatMessage) => void
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
              recallingMessageId={props.recallingMessageId}
              onRecallMessage={props.onRecallMessage}
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
  recallingMessageId: number | null
  onRecallMessage: (message: ChatMessage) => void
}

function MessageBubble(props: MessageBubbleProps) {
  const { t } = useTranslation()
  const revoked = props.message.revoked_at > 0
  const canRecall = props.mine && !revoked
  const recalling = props.recallingMessageId === props.message.id
  const receiptSummary = getMessageReceiptSummary(
    props.message,
    props.conversation
  )
  const showGroupReceipts = Boolean(
    props.conversation?.type === 'group' && receiptSummary.totalRecipients > 0
  )
  const readLabel = getMessageReadLabel(
    props.message,
    props.conversation,
    props.currentUserId
  )
  const readLabelText = readLabel === 'read' ? t('Read') : t('Unread')

  const handleCopy = (): void => {
    if (typeof navigator === 'undefined' || !navigator.clipboard) return
    void navigator.clipboard.writeText(props.message.body).then(() => {
      toast.success(t('Message copied'))
    })
  }

  const handleRecall = (): void => {
    props.onRecallMessage(props.message)
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
          <div
            className={cn(
              'leading-relaxed break-words whitespace-pre-wrap',
              revoked &&
                (props.mine
                  ? 'text-primary-foreground/75 italic'
                  : 'text-muted-foreground italic')
            )}
          >
            {revoked ? t('This message was recalled') : props.message.body}
          </div>
        </div>
        <div
          className={cn(
            'flex items-center gap-2 px-1 text-[11px]',
            props.mine ? 'justify-end' : 'justify-start'
          )}
        >
          {showGroupReceipts ? (
            <MessageReceiptBadges summary={receiptSummary} />
          ) : (
            <span
              className={cn(
                'flex items-center gap-1',
                readLabel === 'read'
                  ? 'text-emerald-600 dark:text-emerald-400'
                  : 'text-muted-foreground'
              )}
            >
              {readLabel === 'read' ? (
                <CheckCheck className='h-3.5 w-3.5' />
              ) : (
                <Check className='h-3.5 w-3.5' />
              )}
              {readLabelText}
            </span>
          )}
          {!revoked && (
            <Button
              type='button'
              variant='ghost'
              size='sm'
              className='h-7 gap-1 px-2 text-xs opacity-0 transition-opacity group-hover:opacity-100'
              onClick={handleCopy}
            >
              <Copy className='h-3.5 w-3.5' />
              {t('Copy')}
            </Button>
          )}
          {canRecall && (
            <Button
              type='button'
              variant='ghost'
              size='sm'
              className='h-7 gap-1 px-2 text-xs opacity-0 transition-opacity group-hover:opacity-100'
              onClick={handleRecall}
              disabled={recalling}
            >
              {recalling ? (
                <Loader2 className='h-3.5 w-3.5 animate-spin' />
              ) : (
                <Undo2 className='h-3.5 w-3.5' />
              )}
              {t('Recall')}
            </Button>
          )}
        </div>
      </div>
    </div>
  )
}

interface MessageReceiptBadgesProps {
  summary: {
    readUsers: ChatUser[]
    unreadUsers: ChatUser[]
  }
}

function MessageReceiptBadges(props: MessageReceiptBadgesProps) {
  const { t } = useTranslation()

  return (
    <div className='flex items-center gap-1.5'>
      <ReceiptPopover
        icon='read'
        title={t('Read by')}
        users={props.summary.readUsers}
        label={t('Read {{count}}', { count: props.summary.readUsers.length })}
      />
      <ReceiptPopover
        icon='unread'
        title={t('Unread by')}
        users={props.summary.unreadUsers}
        label={t('Unread {{count}}', {
          count: props.summary.unreadUsers.length,
        })}
      />
    </div>
  )
}

interface ReceiptPopoverProps {
  icon: 'read' | 'unread'
  title: string
  users: ChatUser[]
  label: string
}

function ReceiptPopover(props: ReceiptPopoverProps) {
  const Icon = props.icon === 'read' ? Eye : EyeOff

  return (
    <Popover>
      <PopoverTrigger
        render={
          <button
            type='button'
            className={cn(
              'hover:bg-muted flex items-center gap-1 rounded-full px-2 py-1 transition-colors',
              props.icon === 'read'
                ? 'text-emerald-600 dark:text-emerald-400'
                : 'text-muted-foreground'
            )}
          />
        }
      >
        <Icon className='h-3.5 w-3.5' />
        {props.label}
      </PopoverTrigger>
      <PopoverContent align='start' className='w-64'>
        <PopoverTitle className='text-sm'>{props.title}</PopoverTitle>
        <ReceiptUserList users={props.users} />
      </PopoverContent>
    </Popover>
  )
}

interface ReceiptUserListProps {
  users: ChatUser[]
}

function ReceiptUserList(props: ReceiptUserListProps) {
  const { t } = useTranslation()

  if (props.users.length === 0) {
    return (
      <div className='text-muted-foreground rounded-lg border border-dashed p-3 text-center text-xs'>
        {t('No users')}
      </div>
    )
  }

  return (
    <div className='max-h-60 space-y-1 overflow-y-auto'>
      {props.users.map((user) => (
        <div key={user.id} className='flex items-center gap-2 rounded-lg p-1.5'>
          <Avatar size='sm' className='shrink-0'>
            <AvatarFallback className='bg-muted text-[10px]'>
              {getChatUserInitial(user)}
            </AvatarFallback>
          </Avatar>
          <div className='min-w-0'>
            <div className='truncate text-xs font-medium'>
              {getChatUserDisplayName(user)}
            </div>
            <div className='text-muted-foreground truncate text-[11px]'>
              @{user.username}
            </div>
          </div>
        </div>
      ))}
    </div>
  )
}
