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
  Loader2,
  MessageCircle,
  RotateCcw,
  Search,
  UserRound,
  Users,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { getUserAvatarStyle } from '@/lib/avatar'
import { cn } from '@/lib/utils'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  formatChatTime,
  getChatUserDisplayName,
  getChatUserInitial,
  getConversationInitial,
  getConversationTitle,
  type SidebarTab,
} from '../lib/format'
import type { ChatConversation, ChatUser } from '../types'

interface ChatSidebarProps {
  activeTab: SidebarTab
  users: ChatUser[]
  conversations: ChatConversation[]
  activeUserId: number | null
  activeConversationId: number | null
  loadingUsers: boolean
  loadingConversations: boolean
  searchText: string
  openingUserId: number | null
  totalUnreadCount: number
  onTabChange: (value: SidebarTab) => void
  onSearchChange: (value: string) => void
  onSelectUser: (user: ChatUser) => void
  onSelectConversation: (conversation: ChatConversation) => void
  onRefresh: () => void
}

export function ChatSidebar(props: ChatSidebarProps) {
  const { t } = useTranslation()

  return (
    <Card className='border-border/80 bg-background flex h-full min-h-0 flex-col overflow-hidden shadow-sm'>
      <CardHeader className='space-y-3 border-b pb-3'>
        <div className='flex items-center justify-between gap-3'>
          <CardTitle className='flex items-center gap-2 text-base'>
            <MessageCircle className='h-4 w-4' />
            {t('Messages')}
            {props.totalUnreadCount > 0 && (
              <Badge className='rounded-full px-1.5 py-0 text-[10px]'>
                {props.totalUnreadCount}
              </Badge>
            )}
          </CardTitle>
          <Button
            type='button'
            variant='ghost'
            size='icon-sm'
            onClick={props.onRefresh}
            aria-label={t('Refresh')}
          >
            <RotateCcw className='h-4 w-4' />
          </Button>
        </div>
        <div className='bg-muted grid grid-cols-2 rounded-xl p-1'>
          <TabButton
            active={props.activeTab === 'users'}
            label={t('Users')}
            onClick={() => props.onTabChange('users')}
          />
          <TabButton
            active={props.activeTab === 'conversations'}
            label={t('Conversations')}
            badge={props.totalUnreadCount}
            onClick={() => props.onTabChange('conversations')}
          />
        </div>
        <div className='relative'>
          <Search className='text-muted-foreground absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2' />
          <Input
            value={props.searchText}
            onChange={(event) => props.onSearchChange(event.target.value)}
            placeholder={
              props.activeTab === 'users'
                ? t('Search users')
                : t('Search conversations')
            }
            className='pl-9'
          />
        </div>
      </CardHeader>
      <CardContent className='min-h-0 flex-1 p-0'>
        {props.activeTab === 'users' ? (
          <UserList
            users={props.users}
            activeUserId={props.activeUserId}
            loading={props.loadingUsers}
            openingUserId={props.openingUserId}
            onSelect={props.onSelectUser}
          />
        ) : (
          <ConversationList
            conversations={props.conversations}
            activeConversationId={props.activeConversationId}
            loading={props.loadingConversations}
            onSelect={props.onSelectConversation}
          />
        )}
      </CardContent>
    </Card>
  )
}

interface TabButtonProps {
  active: boolean
  label: string
  badge?: number
  onClick: () => void
}

function TabButton(props: TabButtonProps) {
  return (
    <button
      type='button'
      onClick={props.onClick}
      className={cn(
        'flex items-center justify-center gap-2 rounded-lg px-2 py-1.5 text-sm font-medium transition-colors',
        props.active ? 'bg-background shadow-sm' : 'text-muted-foreground'
      )}
    >
      <span>{props.label}</span>
      {Boolean(props.badge) && (
        <span className='bg-primary text-primary-foreground rounded-full px-1.5 text-[10px] leading-4'>
          {props.badge}
        </span>
      )}
    </button>
  )
}

interface UserListProps {
  users: ChatUser[]
  activeUserId: number | null
  loading: boolean
  openingUserId: number | null
  onSelect: (user: ChatUser) => void
}

function UserList(props: UserListProps) {
  const { t } = useTranslation()

  if (props.loading) {
    return <LoadingState label={t('Loading users...')} />
  }

  if (props.users.length === 0) {
    return <EmptyState icon='user' label={t('No users found')} />
  }

  return (
    <ScrollArea className='h-full'>
      <div className='space-y-1 p-2'>
        {props.users.map((user) => (
          <UserListItem
            key={user.id}
            user={user}
            active={props.activeUserId === user.id}
            opening={props.openingUserId === user.id}
            onSelect={props.onSelect}
          />
        ))}
      </div>
    </ScrollArea>
  )
}

interface UserListItemProps {
  user: ChatUser
  active: boolean
  opening: boolean
  onSelect: (user: ChatUser) => void
}

function UserListItem(props: UserListItemProps) {
  const displayName = getChatUserDisplayName(props.user)

  return (
    <button
      type='button'
      onClick={() => props.onSelect(props.user)}
      className={cn(
        'hover:bg-muted/60 flex w-full items-center gap-3 rounded-xl px-3 py-2 text-left transition-colors',
        props.active && 'bg-primary/10 text-primary hover:bg-primary/10'
      )}
    >
      <Avatar size='sm' className='shrink-0'>
        <AvatarFallback
          className='text-xs font-semibold'
          style={getUserAvatarStyle(displayName)}
        >
          {getChatUserInitial(props.user)}
        </AvatarFallback>
      </Avatar>
      <div className='min-w-0 flex-1'>
        <div className='truncate text-sm font-medium'>
          @{props.user.username}
        </div>
        {displayName !== props.user.username && (
          <div className='text-muted-foreground truncate text-xs'>
            {displayName}
          </div>
        )}
      </div>
      {props.opening && <Loader2 className='h-4 w-4 animate-spin' />}
    </button>
  )
}

interface ConversationListProps {
  conversations: ChatConversation[]
  activeConversationId: number | null
  loading: boolean
  onSelect: (conversation: ChatConversation) => void
}

function ConversationList(props: ConversationListProps) {
  const { t } = useTranslation()

  if (props.loading) {
    return <LoadingState label={t('Loading conversations...')} />
  }

  if (props.conversations.length === 0) {
    return <EmptyState icon='conversation' label={t('No conversations yet')} />
  }

  return (
    <ScrollArea className='h-full'>
      <div className='space-y-1 p-2'>
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
  onSelect: (conversation: ChatConversation) => void
}

function ConversationListItem(props: ConversationListItemProps) {
  const { t } = useTranslation()
  const title = getConversationTitle(props.conversation)
  const avatarName = props.conversation.peer
    ? getChatUserDisplayName(props.conversation.peer)
    : title

  return (
    <button
      type='button'
      onClick={() => props.onSelect(props.conversation)}
      className={cn(
        'hover:bg-muted/60 flex w-full items-center gap-3 rounded-xl px-3 py-2 text-left transition-colors',
        props.active && 'bg-primary/10 text-primary hover:bg-primary/10'
      )}
    >
      <Avatar size='sm' className='shrink-0'>
        <AvatarFallback
          className='text-xs font-semibold'
          style={getUserAvatarStyle(avatarName)}
        >
          {getConversationInitial(props.conversation)}
        </AvatarFallback>
      </Avatar>
      <div className='min-w-0 flex-1'>
        <div className='truncate text-sm font-medium'>{t(title)}</div>
        <div className='text-muted-foreground truncate text-xs'>
          {props.conversation.last_message_at
            ? formatChatTime(props.conversation.last_message_at)
            : t('No messages yet')}
        </div>
      </div>
      {props.conversation.unread_count > 0 && (
        <Badge className='rounded-full px-1.5 py-0 text-[10px]'>
          {props.conversation.unread_count}
        </Badge>
      )}
    </button>
  )
}

interface LoadingStateProps {
  label: string
}

function LoadingState(props: LoadingStateProps) {
  return (
    <div className='text-muted-foreground flex items-center gap-2 p-4 text-sm'>
      <Loader2 className='h-4 w-4 animate-spin' />
      {props.label}
    </div>
  )
}

interface EmptyStateProps {
  icon: 'user' | 'conversation'
  label: string
}

function EmptyState(props: EmptyStateProps) {
  const Icon = props.icon === 'user' ? UserRound : Users
  return (
    <div className='p-6 text-center'>
      <div className='bg-muted mx-auto mb-3 flex size-12 items-center justify-center rounded-2xl'>
        <Icon className='text-muted-foreground h-5 w-5' />
      </div>
      <p className='text-sm font-medium'>{props.label}</p>
    </div>
  )
}
