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
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import type { ChatImagePreview } from '../lib/message-content'

interface ChatImagePreviewDialogProps {
  image: ChatImagePreview | null
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function ChatImagePreviewDialog(props: ChatImagePreviewDialogProps) {
  const { t } = useTranslation()

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='flex max-h-[calc(100dvh-2rem)] flex-col sm:max-w-4xl'>
        <DialogHeader>
          <DialogTitle>{t('Image Preview')}</DialogTitle>
        </DialogHeader>
        <div className='bg-muted/40 flex min-h-0 items-center justify-center overflow-auto rounded-2xl border p-2'>
          {props.image && (
            <img
              src={props.image.src}
              alt={props.image.alt || t('Image Preview')}
              className='max-h-[72dvh] max-w-full rounded-xl object-contain shadow-sm'
            />
          )}
        </div>
        <DialogFooter>
          <Button variant='outline' onClick={() => props.onOpenChange(false)}>
            {t('Cancel')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
