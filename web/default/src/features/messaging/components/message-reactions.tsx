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
import { useState } from 'react'
import { SmilePlus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import {
  Popover,
  PopoverContent,
  PopoverTitle,
  PopoverTrigger,
} from '@/components/ui/popover'
import { AVAILABLE_CHAT_REACTIONS } from '../lib/message-content'
import type { ChatMessageReaction } from '../types'

interface MessageReactionsProps {
  reactions: ChatMessageReaction[]
  mine: boolean
  disabled: boolean
  onReact: (emoji: string) => void
}

export function MessageReactions(props: MessageReactionsProps) {
  return (
    <div
      className={cn(
        'flex flex-wrap items-center gap-1 px-1',
        props.mine ? 'justify-end' : 'justify-start'
      )}
    >
      {props.reactions.map((reaction) => (
        <ReactionCountButton
          key={reaction.emoji}
          reaction={reaction}
          disabled={props.disabled}
          onReact={props.onReact}
        />
      ))}
      <ReactionPicker
        compact={props.reactions.length > 0}
        disabled={props.disabled}
        onReact={props.onReact}
      />
    </div>
  )
}

interface ReactionCountButtonProps {
  reaction: ChatMessageReaction
  disabled: boolean
  onReact: (emoji: string) => void
}

function ReactionCountButton(props: ReactionCountButtonProps) {
  const { t } = useTranslation()

  return (
    <button
      type='button'
      disabled={props.disabled}
      onClick={() => props.onReact(props.reaction.emoji)}
      className={cn(
        'focus-visible:ring-ring inline-flex h-7 items-center gap-1 rounded-full border px-2 text-xs transition-colors focus-visible:ring-2 focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-60',
        props.reaction.reacted_by_me
          ? 'border-[#07c160]/60 bg-[#07c160]/10 text-[#058f47] dark:text-[#78c957]'
          : 'bg-background/80 text-muted-foreground hover:bg-muted'
      )}
      aria-label={t('React with {{emoji}}', {
        emoji: props.reaction.emoji,
      })}
    >
      <span className='text-sm leading-none'>{props.reaction.emoji}</span>
      <span className='tabular-nums'>{props.reaction.count}</span>
    </button>
  )
}

interface ReactionPickerProps {
  compact: boolean
  disabled: boolean
  onReact: (emoji: string) => void
}

function ReactionPicker(props: ReactionPickerProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)

  const handleReact = (emoji: string): void => {
    props.onReact(emoji)
    setOpen(false)
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={
          <button
            type='button'
            disabled={props.disabled}
            className={cn(
              'hover:bg-muted focus-visible:ring-ring inline-flex items-center justify-center rounded-full border bg-background/80 text-muted-foreground transition-colors focus-visible:ring-2 focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-60',
              props.compact
                ? 'h-7 w-7 opacity-0 group-hover:opacity-100 focus:opacity-100'
                : 'h-7 gap-1 px-2 text-xs opacity-0 group-hover:opacity-100 focus:opacity-100'
            )}
            aria-label={t('Add reaction')}
          />
        }
      >
        <SmilePlus className='h-3.5 w-3.5' />
        {!props.compact && <span>{t('React')}</span>}
      </PopoverTrigger>
      <PopoverContent align='center' side='top' className='w-72'>
        <PopoverTitle className='text-sm'>{t('Add reaction')}</PopoverTitle>
        <div className='grid grid-cols-8 gap-1.5'>
          {AVAILABLE_CHAT_REACTIONS.map((reaction) => (
            <button
              key={reaction.id}
              type='button'
              disabled={props.disabled}
              onClick={() => handleReact(reaction.emoji)}
              className='hover:bg-muted focus-visible:ring-ring flex size-8 items-center justify-center rounded-lg text-xl leading-none transition-transform hover:scale-110 focus-visible:ring-2 focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-60'
              aria-label={t('React with {{emoji}}', {
                emoji: reaction.emoji,
              })}
            >
              {reaction.emoji}
            </button>
          ))}
        </div>
      </PopoverContent>
    </Popover>
  )
}
