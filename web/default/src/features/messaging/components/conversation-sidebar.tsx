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
import {
  Hash,
  Inbox,
  Loader2,
  MessageCircle,
  RotateCcw,
  Search,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  type ConversationFilter,
  formatChatTime,
  getConversationFallbackTitle,
} from '../lib/format'
import type { ChatConversation } from '../types'
import { ConversationAvatar } from './conversation-avatar'

interface ConversationSidebarProps {
  conversations: ChatConversation[]
  allConversationCount: number
  activeConversationId: number | null
  loading: boolean
  searchText: string
  filter: ConversationFilter
  directCount: number
  groupCount: number
  onSearchChange: (value: string) => void
  onFilterChange: (value: ConversationFilter) => void
  onSelect: (conversationId: number) => void
  onRefresh: () => void
}

export function ConversationSidebar(props: ConversationSidebarProps) {
  const { t } = useTranslation()

  return (
    <Card className='border-border/80 bg-background/95 min-h-0 flex-1 overflow-hidden shadow-sm'>
      <CardHeader className='space-y-4 pb-3'>
        <div className='flex items-center justify-between gap-3'>
          <CardTitle className='flex items-center gap-2 text-base'>
            <MessageCircle className='h-4 w-4' />
            {t('Conversation inbox')}
          </CardTitle>
          <Button
            type='button'
            variant='ghost'
            size='icon-sm'
            onClick={props.onRefresh}
            aria-label={t('Refresh conversations')}
          >
            <RotateCcw className='h-4 w-4' />
          </Button>
        </div>
        <div className='relative'>
          <Search className='text-muted-foreground absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2' />
          <Input
            value={props.searchText}
            onChange={(event) => props.onSearchChange(event.target.value)}
            placeholder={t('Search chats or IDs')}
            className='pl-9'
          />
        </div>
        <div className='grid grid-cols-3 gap-2'>
          <FilterButton
            active={props.filter === 'all'}
            label={t('All')}
            count={props.allConversationCount}
            onClick={() => props.onFilterChange('all')}
          />
          <FilterButton
            active={props.filter === 'direct'}
            label={t('Direct')}
            count={props.directCount}
            onClick={() => props.onFilterChange('direct')}
          />
          <FilterButton
            active={props.filter === 'group'}
            label={t('Groups')}
            count={props.groupCount}
            onClick={() => props.onFilterChange('group')}
          />
        </div>
      </CardHeader>
      <CardContent className='min-h-0 p-0'>
        <ConversationList
          conversations={props.conversations}
          activeConversationId={props.activeConversationId}
          loading={props.loading}
          onSelect={props.onSelect}
        />
      </CardContent>
    </Card>
  )
}

interface FilterButtonProps {
  active: boolean
  label: string
  count: number
  onClick: () => void
}

function FilterButton(props: FilterButtonProps) {
  return (
    <button
      type='button'
      onClick={props.onClick}
      className={cn(
        'rounded-xl border px-2 py-2 text-left transition-colors',
        props.active
          ? 'border-primary/40 bg-primary/10 text-primary'
          : 'hover:bg-muted/60 bg-muted/20'
      )}
    >
      <span className='block text-xs font-medium'>{props.label}</span>
      <span className='text-sm font-semibold tabular-nums'>{props.count}</span>
    </button>
  )
}

interface ConversationListProps {
  conversations: ChatConversation[]
  activeConversationId: number | null
  loading: boolean
  onSelect: (conversationId: number) => void
}

function ConversationList(props: ConversationListProps) {
  const { t } = useTranslation()

  if (props.loading) {
    return (
      <div className='text-muted-foreground flex items-center gap-2 p-4 text-sm'>
        <Loader2 className='h-4 w-4 animate-spin' />
        {t('Loading conversations...')}
      </div>
    )
  }

  if (props.conversations.length === 0) {
    return (
      <div className='p-6 text-center'>
        <div className='bg-muted mx-auto mb-3 flex size-12 items-center justify-center rounded-2xl'>
          <Inbox className='text-muted-foreground h-5 w-5' />
        </div>
        <p className='text-sm font-medium'>{t('No matching conversations')}</p>
        <p className='text-muted-foreground mt-1 text-xs'>
          {t('Start a direct chat or create a group workspace below.')}
        </p>
      </div>
    )
  }

  return (
    <ScrollArea className='h-[26rem] lg:h-[calc(100vh-26rem)] xl:h-[calc(100vh-25rem)]'>
      <div className='space-y-2 p-3'>
        {props.conversations.map((conversation) => (
          <ConversationListItem
            key={conversation.id}
            conversation={conversation}
            active={props.activeConversationId === conversation.id}
            onSelect={props.onSelect}
          />
        ))}
      </div>
    </ScrollArea>
  )
}

interface ConversationListItemProps {
  conversation: ChatConversation
  active: boolean
  onSelect: (conversationId: number) => void
}

function ConversationListItem(props: ConversationListItemProps) {
  const { t } = useTranslation()
  const title = getConversationFallbackTitle(props.conversation)

  return (
    <button
      type='button'
      onClick={() => props.onSelect(props.conversation.id)}
      className={cn(
        'group hover:border-primary/30 hover:bg-muted/50 w-full rounded-2xl border p-3 text-left transition-all hover:-translate-y-0.5 hover:shadow-sm',
        props.active
          ? 'border-primary/40 bg-primary/10 shadow-sm'
          : 'border-border/70 bg-card/70'
      )}
    >
      <div className='flex items-start gap-3'>
        <ConversationAvatar conversation={props.conversation} />
        <div className='min-w-0 flex-1'>
          <div className='flex items-center justify-between gap-2'>
            <span className='truncate text-sm font-semibold'>{title}</span>
            <Badge
              variant='outline'
              className='shrink-0 rounded-full text-[10px]'
            >
              {t(props.conversation.type)}
            </Badge>
          </div>
          <div className='text-muted-foreground mt-1 flex items-center gap-1 text-xs'>
            <Hash className='h-3 w-3' />
            <span>{props.conversation.id}</span>
            <span>·</span>
            <span>{formatChatTime(props.conversation.last_message_at)}</span>
          </div>
          <div className='mt-3 flex items-center justify-between gap-2'>
            <span className='text-muted-foreground truncate text-xs'>
              {props.conversation.last_message_id > 0
                ? t('Message #{{id}}', {
                    id: props.conversation.last_message_id,
                  })
                : t('No messages yet')}
            </span>
            <span className='h-2 w-2 rounded-full bg-emerald-500/80 shadow-[0_0_0_3px_rgba(16,185,129,0.14)]' />
          </div>
        </div>
      </div>
    </button>
  )
}
