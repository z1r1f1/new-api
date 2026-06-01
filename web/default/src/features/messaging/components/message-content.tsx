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
import { cn } from '@/lib/utils'
import { extractMessageContentParts } from '../lib/message-content'

interface MessageContentProps {
  body: string
  mine: boolean
}

export function MessageContent(props: MessageContentProps) {
  const parts = extractMessageContentParts(props.body)

  return (
    <div className='flex flex-col gap-2'>
      {parts.map((part, index) => {
        if (part.type === 'text') {
          if (!part.text) return null
          return (
            <span key={`text-${index}`} className='whitespace-pre-wrap'>
              {part.text}
            </span>
          )
        }
        return (
          <a
            key={`image-${index}`}
            href={part.src}
            target='_blank'
            rel='noreferrer'
            className='block max-w-full'
          >
            <img
              src={part.src}
              alt={part.alt || 'pasted-image'}
              loading='lazy'
              className={cn(
                'max-h-80 max-w-full rounded-2xl border object-contain shadow-sm',
                props.mine ? 'border-primary-foreground/25' : 'border-border'
              )}
            />
          </a>
        )
      })}
    </div>
  )
}
