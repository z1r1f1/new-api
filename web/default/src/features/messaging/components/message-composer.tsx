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
import { Loader2, Send } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'

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

  return (
    <div className='border-border/80 bg-background border-t p-3 lg:p-4'>
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
