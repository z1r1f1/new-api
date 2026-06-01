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
import { AlertCircle, AlertTriangle, Settings } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { useAuthStore } from '@/stores/auth-store'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { MESSAGE_STATUS } from '../constants'
import type { Message } from '../types'

interface MessageErrorProps {
  message: Message
  actions?: ReactNode
  className?: string
}

/**
 * Display error messages using Alert component
 * Following ai-elements pattern for error handling
 */
export function MessageError(props: MessageErrorProps) {
  const { t } = useTranslation()
  const user = useAuthStore((s) => s.auth.user)
  const isAdmin = user?.role != null && user.role >= 10

  if (props.message.status !== MESSAGE_STATUS.ERROR) {
    return null
  }

  const errorContent =
    props.message.versions[0]?.content || t('An unknown error occurred')

  if (props.message.errorCode === 'model_price_error') {
    return (
      <Alert variant='default' className={props.className}>
        <AlertTriangle className='text-orange-500' />
        <AlertTitle>{t('Model Price Not Configured')}</AlertTitle>
        <AlertDescription className='space-y-2'>
          <p>{errorContent}</p>
          {isAdmin && (
            <Button
              variant='outline'
              size='sm'
              onClick={() =>
                window.open('/system-settings/billing/model-pricing', '_blank')
              }
            >
              <Settings className='mr-1 h-3.5 w-3.5' />
              {t('Go to Settings')}
            </Button>
          )}
          {props.actions}
        </AlertDescription>
      </Alert>
    )
  }

  return (
    <Alert variant='destructive' className={props.className}>
      <AlertCircle />
      <AlertTitle>{t('Error')}</AlertTitle>
      <AlertDescription className='space-y-2'>
        <p>{errorContent}</p>
        {props.actions}
      </AlertDescription>
    </Alert>
  )
}
