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
import { Loader2, RotateCcw, Search, UserRound, Users } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import { getChatUserDisplayName, getChatUserInitial } from '../lib/format'
import type { ChatUser } from '../types'

interface UserSidebarProps {
  users: ChatUser[]
  activeUserId: number | null
  loading: boolean
  searchText: string
  openingUserId: number | null
  onSearchChange: (value: string) => void
  onSelect: (user: ChatUser) => void
  onRefresh: () => void
}

export function UserSidebar(props: UserSidebarProps) {
  const { t } = useTranslation()

  return (
    <Card className='border-border/80 bg-background flex h-full min-h-0 flex-col overflow-hidden shadow-sm'>
      <CardHeader className='space-y-3 border-b pb-3'>
        <div className='flex items-center justify-between gap-3'>
          <CardTitle className='flex items-center gap-2 text-base'>
            <Users className='h-4 w-4' />
            {t('Users')}
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
        <div className='relative'>
          <Search className='text-muted-foreground absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2' />
          <Input
            value={props.searchText}
            onChange={(event) => props.onSearchChange(event.target.value)}
            placeholder={t('Search users')}
            className='pl-9'
          />
        </div>
      </CardHeader>
      <CardContent className='min-h-0 flex-1 p-0'>
        <UserList
          users={props.users}
          activeUserId={props.activeUserId}
          loading={props.loading}
          openingUserId={props.openingUserId}
          onSelect={props.onSelect}
        />
      </CardContent>
    </Card>
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
    return (
      <div className='text-muted-foreground flex items-center gap-2 p-4 text-sm'>
        <Loader2 className='h-4 w-4 animate-spin' />
        {t('Loading users...')}
      </div>
    )
  }

  if (props.users.length === 0) {
    return (
      <div className='p-6 text-center'>
        <div className='bg-muted mx-auto mb-3 flex size-12 items-center justify-center rounded-2xl'>
          <UserRound className='text-muted-foreground h-5 w-5' />
        </div>
        <p className='text-sm font-medium'>{t('No users found')}</p>
      </div>
    )
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
        <AvatarFallback className='bg-muted text-xs'>
          {getChatUserInitial(props.user)}
        </AvatarFallback>
      </Avatar>
      <div className='min-w-0 flex-1'>
        <div className='truncate text-sm font-medium'>@{props.user.username}</div>
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
