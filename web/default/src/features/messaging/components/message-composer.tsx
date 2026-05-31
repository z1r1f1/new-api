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
import { Loader2, Paperclip, Send, Smile } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { QUICK_REPLIES } from '../lib/format'

interface MessageComposerProps {
  value: string
  active: boolean
  sending: boolean
  maxLength: number
  onChange: (value: string) => void
  onSend: () => void
}

export function MessageComposer(props: MessageComposerProps) {
  const { t } = useTranslation()
  const remaining = props.maxLength - props.value.length

  const handleQuickReply = (value: string): void => {
    const nextValue = props.value.trim()
      ? `${props.value.trim()}\n${t(value)}`
      : t(value)
    props.onChange(nextValue)
  }

  return (
    <div className='border-border/80 bg-background/95 border-t p-3 lg:p-4'>
      <div className='mb-3 flex flex-wrap items-center gap-2'>
        {QUICK_REPLIES.map((reply) => (
          <Button
            key={reply}
            type='button'
            variant='outline'
            size='sm'
            disabled={!props.active || props.sending}
            onClick={() => handleQuickReply(reply)}
            className='h-8 rounded-full text-xs'
          >
            {t(reply)}
          </Button>
        ))}
      </div>
      <div className='bg-muted/20 focus-within:border-primary/50 focus-within:bg-background rounded-3xl border p-2'>
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
        <div className='flex flex-col gap-2 border-t pt-2 sm:flex-row sm:items-center sm:justify-between'>
          <div className='text-muted-foreground flex flex-wrap items-center gap-3 px-1 text-xs'>
            <span className='flex items-center gap-1'>
              <Paperclip className='h-3.5 w-3.5' />
              {t('Files coming soon')}
            </span>
            <span className='flex items-center gap-1'>
              <Smile className='h-3.5 w-3.5' />
              {t('Shift Enter for newline')}
            </span>
            <span
              className={cn(
                'tabular-nums',
                remaining < 0 && 'text-destructive'
              )}
            >
              {t('{{count}} characters left', { count: remaining })}
            </span>
          </div>
          <Button
            type='button'
            onClick={props.onSend}
            disabled={!props.active || props.sending || !props.value.trim()}
            className='gap-2 rounded-full'
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
