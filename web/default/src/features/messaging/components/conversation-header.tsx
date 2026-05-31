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
import { MessageCircle, RotateCcw, Search } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import type { ChatConversation } from '../types'
import { ConversationAvatar } from './conversation-avatar'
import { ConversationTitle } from './conversation-title'
import { RealtimeStatusBadge } from './realtime-status-badge'

interface ConversationHeaderProps {
  conversation: ChatConversation | null
  realtimeStatus: string
  messageCount: number
  searchText: string
  onSearchChange: (value: string) => void
  onRefresh: () => void
}

export function ConversationHeader(props: ConversationHeaderProps) {
  const { t } = useTranslation()

  return (
    <CardHeader className='border-border/80 bg-muted/10 border-b'>
      <div className='flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between'>
        <div className='flex min-w-0 items-center gap-3'>
          {props.conversation ? (
            <ConversationAvatar conversation={props.conversation} />
          ) : (
            <div className='bg-muted flex size-10 items-center justify-center rounded-full'>
              <MessageCircle className='text-muted-foreground h-5 w-5' />
            </div>
          )}
          <div className='min-w-0'>
            <CardTitle className='truncate text-base'>
              {props.conversation ? (
                <ConversationTitle conversation={props.conversation} />
              ) : (
                t('Select a conversation')
              )}
            </CardTitle>
            <div className='text-muted-foreground mt-1 flex flex-wrap items-center gap-2 text-xs'>
              {props.conversation ? (
                <>
                  <span>
                    {t('Conversation ID')}: {props.conversation.id}
                  </span>
                  <span>·</span>
                  <span>{t(props.conversation.type)}</span>
                  <span>·</span>
                  <span>
                    {t('{{count}} messages', { count: props.messageCount })}
                  </span>
                </>
              ) : (
                <span>
                  {t(
                    'Choose a conversation from the inbox to open the workspace.'
                  )}
                </span>
              )}
            </div>
          </div>
        </div>
        <div className='flex flex-col gap-2 sm:flex-row sm:items-center'>
          <div className='relative min-w-0 sm:w-64'>
            <Search className='text-muted-foreground absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2' />
            <Input
              value={props.searchText}
              onChange={(event) => props.onSearchChange(event.target.value)}
              placeholder={t('Search in conversation')}
              disabled={!props.conversation}
              className='pl-9'
            />
          </div>
          <Button
            type='button'
            variant='outline'
            onClick={props.onRefresh}
            className='gap-2'
          >
            <RotateCcw className='h-4 w-4' />
            {t('Refresh')}
          </Button>
          <RealtimeStatusBadge status={props.realtimeStatus} />
        </div>
      </div>
    </CardHeader>
  )
}
