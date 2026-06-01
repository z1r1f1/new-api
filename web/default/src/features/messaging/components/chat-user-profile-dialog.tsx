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
import { useQuery } from '@tanstack/react-query'
import { Loader2, MessageCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { getUserAvatarStyle } from '@/lib/avatar'
import { formatNumber, formatQuota, formatTimestamp } from '@/lib/format'
import { getRoleLabelKey } from '@/lib/roles'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { getUser } from '@/features/users/api'
import { USER_STATUS } from '@/features/users/constants'
import type { User } from '@/features/users/types'
import { getChatUserDisplayName, getChatUserInitial } from '../lib/format'
import type { ChatUser } from '../types'

interface ChatUserProfileDialogProps {
  user: ChatUser | null
  open: boolean
  canViewDetails: boolean
  currentUserId: number | null
  openingDirect: boolean
  onOpenChange: (open: boolean) => void
  onStartDirectChat: (user: ChatUser) => void
}

interface DetailRowProps {
  label: string
  value: string
}

export function ChatUserProfileDialog(props: ChatUserProfileDialogProps) {
  const { t } = useTranslation()
  const userDetailsQuery = useQuery({
    queryKey: ['messaging', 'user-profile', props.user?.id ?? 0],
    queryFn: () => getUser(props.user?.id ?? 0),
    enabled: props.open && props.canViewDetails && Boolean(props.user?.id),
  })

  if (!props.user) return null

  const chatUser = props.user
  const displayName = getChatUserDisplayName(chatUser)
  const userDetails = userDetailsQuery.data?.data
  const role = userDetails?.role ?? chatUser.role
  const canStartDirectChat = props.currentUserId !== chatUser.id

  const handleStartDirectChat = (): void => {
    props.onStartDirectChat(chatUser)
  }

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='sm:max-w-lg'>
        <DialogHeader>
          <DialogTitle>{t('User profile')}</DialogTitle>
          <DialogDescription>
            {props.canViewDetails
              ? t('User details are loaded from the admin user API.')
              : t('Only admins can view user details.')}
          </DialogDescription>
        </DialogHeader>

        <div className='space-y-4'>
          <div className='flex items-center gap-3 rounded-2xl border p-3'>
            <Avatar size='lg' className='shadow-sm'>
              <AvatarFallback
                className='text-sm font-semibold'
                style={getUserAvatarStyle(displayName)}
              >
                {getChatUserInitial(chatUser)}
              </AvatarFallback>
            </Avatar>
            <div className='min-w-0 flex-1'>
              <div className='truncate text-base font-semibold'>
                {displayName}
              </div>
              <div className='text-muted-foreground truncate text-sm'>
                @{chatUser.username}
              </div>
              {props.canViewDetails && (
                <div className='mt-2 flex flex-wrap items-center gap-2'>
                  <Badge variant='outline'>{t(getRoleLabelKey(role))}</Badge>
                  <Badge variant='secondary'>ID: {chatUser.id}</Badge>
                </div>
              )}
            </div>
          </div>

          {!props.canViewDetails && (
            <Alert>
              <AlertDescription>
                {t('Only admins can view user details.')}
              </AlertDescription>
            </Alert>
          )}

          {props.canViewDetails && userDetailsQuery.isPending && (
            <div className='text-muted-foreground flex items-center gap-2 rounded-2xl border border-dashed p-3 text-sm'>
              <Loader2 className='h-4 w-4 animate-spin' />
              {t('Loading user details...')}
            </div>
          )}

          {props.canViewDetails && userDetailsQuery.isError && (
            <Alert variant='destructive'>
              <AlertDescription>
                {t('Failed to load user details')}
              </AlertDescription>
            </Alert>
          )}

          {props.canViewDetails && userDetails && (
            <div className='grid gap-2 rounded-2xl border p-3 sm:grid-cols-2'>
              <DetailRow label={t('Email')} value={userDetails.email || '-'} />
              <DetailRow label={t('Group')} value={userDetails.group || '-'} />
              <DetailRow
                label={t('Status')}
                value={getUserStatusLabel(userDetails, t)}
              />
              <DetailRow
                label={t('Request Count')}
                value={formatNumber(userDetails.request_count)}
              />
              <DetailRow
                label={t('Quota')}
                value={formatQuota(userDetails.quota)}
              />
              <DetailRow
                label={t('Used Quota')}
                value={formatQuota(userDetails.used_quota)}
              />
              <DetailRow
                label={t('Created At')}
                value={formatOptionalTimestamp(userDetails.created_at)}
              />
              <DetailRow
                label={t('Last Login')}
                value={formatOptionalTimestamp(userDetails.last_login_at)}
              />
              {userDetails.remark && (
                <div className='sm:col-span-2'>
                  <DetailRow label={t('Remark')} value={userDetails.remark} />
                </div>
              )}
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant='outline' onClick={() => props.onOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button
            onClick={handleStartDirectChat}
            disabled={!canStartDirectChat || props.openingDirect}
            className='gap-2'
          >
            {props.openingDirect ? (
              <Loader2 className='h-4 w-4 animate-spin' />
            ) : (
              <MessageCircle className='h-4 w-4' />
            )}
            {t('Private chat')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function DetailRow(props: DetailRowProps) {
  return (
    <div className='bg-muted/40 min-w-0 rounded-xl px-3 py-2'>
      <div className='text-muted-foreground text-[11px]'>{props.label}</div>
      <div className='mt-1 truncate text-sm font-medium' title={props.value}>
        {props.value}
      </div>
    </div>
  )
}

function getUserStatusLabel(
  user: User,
  translate: (key: string) => string
): string {
  if (user.status === USER_STATUS.ENABLED) return translate('Enabled')
  if (user.status === USER_STATUS.DISABLED) return translate('Disabled')
  return translate('Unknown')
}

function formatOptionalTimestamp(timestamp?: number): string {
  if (!timestamp || timestamp <= 0) return '-'
  return formatTimestamp(timestamp)
}
