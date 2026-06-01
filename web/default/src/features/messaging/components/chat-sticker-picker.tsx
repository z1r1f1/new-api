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
import { Sticker } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTitle,
  PopoverTrigger,
} from '@/components/ui/popover'
import type { ChatSticker } from '../lib/message-content'

interface ChatStickerPickerProps {
  stickers: ChatSticker[]
  disabled: boolean
  onSelectSticker: (sticker: ChatSticker) => void
}

export function ChatStickerPicker(props: ChatStickerPickerProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)

  const handleSelectSticker = (sticker: ChatSticker): void => {
    props.onSelectSticker(sticker)
    setOpen(false)
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={
          <Button
            type='button'
            variant='ghost'
            size='icon'
            disabled={props.disabled}
            aria-label={t('Stickers')}
          />
        }
      >
        <Sticker className='h-4 w-4' />
      </PopoverTrigger>
      <PopoverContent align='end' side='top' className='w-80'>
        <PopoverTitle className='text-sm'>{t('Stickers')}</PopoverTitle>
        <div className='grid grid-cols-4 gap-2'>
          {props.stickers.map((sticker) => (
            <button
              key={sticker.id}
              type='button'
              onClick={() => handleSelectSticker(sticker)}
              className={cn(
                'focus-visible:ring-ring flex min-h-16 flex-col items-center justify-center rounded-2xl border bg-gradient-to-br p-2 text-center transition-transform hover:scale-[1.03] focus-visible:ring-2 focus-visible:ring-offset-2',
                sticker.accent
              )}
              aria-label={t('Send {{name}} sticker', {
                name: t(sticker.label),
              })}
            >
              <span className='text-2xl leading-none'>{sticker.emoji}</span>
              <span className='mt-1 text-[11px] font-medium text-slate-700'>
                {t(sticker.label)}
              </span>
            </button>
          ))}
        </div>
      </PopoverContent>
    </Popover>
  )
}
