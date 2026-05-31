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
import { AtSign, Loader2, Send } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { getChatUserDisplayName } from '../lib/format'
import type { ChatUser } from '../types'

interface MessageComposerProps {
  value: string
  active: boolean
  sending: boolean
  maxLength: number
  mentionUsers: ChatUser[]
  onChange: (value: string) => void
  onSend: () => void
}

export function MessageComposer(props: MessageComposerProps) {
  const { t } = useTranslation()
  const remaining = props.maxLength - props.value.length

  const handleMention = (user: ChatUser): void => {
    const mention = `@${user.username} `
    props.onChange(props.value ? `${props.value}${mention}` : mention)
  }

  return (
    <div className='border-border/80 bg-background border-t p-3 lg:p-4'>
      {props.mentionUsers.length > 0 && (
        <div className='mb-2 flex flex-wrap items-center gap-2'>
          <span className='text-muted-foreground flex items-center gap-1 text-xs'>
            <AtSign className='h-3.5 w-3.5' />
            {t('Mention')}
          </span>
          {props.mentionUsers.map((user) => (
            <Button
              key={user.id}
              type='button'
              variant='outline'
              size='sm'
              disabled={!props.active || props.sending}
              className='h-7 rounded-full px-2 text-xs'
              onClick={() => handleMention(user)}
            >
              @{user.username}
              {getChatUserDisplayName(user) !== user.username && (
                <span className='text-muted-foreground ml-1 max-w-20 truncate'>
                  {getChatUserDisplayName(user)}
                </span>
              )}
            </Button>
          ))}
        </div>
      )}
      <div className='focus-within:border-primary/50 rounded-2xl border p-2'>
        <Textarea
          value={props.value}
          onChange={(event) => props.onChange(event.target.value)}
          placeholder={t('Type a message...')}
          disabled={!props.active || props.sending}
          className='min-h-20 resize-none border-0 bg-transparent shadow-none focus-visible:ring-0'
          onKeyDown={(event) => {
            if (event.key === 'Enter' && !event.shiftKey) {
              event.preventDefault()
              props.onSend()
            }
          }}
        />
        <div className='flex items-center justify-between gap-3 border-t pt-2'>
          <span
            className={cn(
              'text-muted-foreground px-1 text-xs tabular-nums',
              remaining < 0 && 'text-destructive'
            )}
          >
            {t('{{count}} characters left', { count: remaining })}
          </span>
          <Button
            type='button'
            onClick={props.onSend}
            disabled={!props.active || props.sending || !props.value.trim()}
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
