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
import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Ban,
  Loader2,
  MessageCircle,
  Power,
  PowerOff,
  Save,
  Volume2,
  VolumeX,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { getUserAvatarStyle } from '@/lib/avatar'
import { formatNumber, formatQuota, formatTimestamp } from '@/lib/format'
import { ROLE, getRoleLabelKey } from '@/lib/roles'
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
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { ConfirmDialog } from '@/components/confirm-dialog'
import {
  getGroups,
  getUser,
  manageUser,
  updateUser,
} from '@/features/users/api'
import { UserQuotaDialog } from '@/features/users/components/user-quota-dialog'
import { USER_STATUS } from '@/features/users/constants'
import type { ManageUserAction, User } from '@/features/users/types'
import { setChatMemberMuted } from '../api'
import { getChatUserDisplayName, getChatUserInitial } from '../lib/format'
import {
  canManageChatProfileUser,
  getConversationMemberMuteState,
} from '../lib/permissions'
import type { ChatConversation, ChatUser } from '../types'

interface ChatUserProfileDialogProps {
  user: ChatUser | null
  conversation: ChatConversation | null
  open: boolean
  canViewDetails: boolean
  currentUserId: number | null
  currentUserRole: number
  openingDirect: boolean
  onOpenChange: (open: boolean) => void
  onStartDirectChat: (user: ChatUser) => void
  onUserUpdated: () => void
  onConversationUpdated: () => void
}

interface DetailRowProps {
  label: string
  value: string
}

type StatusAction = Extract<ManageUserAction, 'enable' | 'disable'>

interface GroupDraft {
  userId: number
  group: string
}

export function ChatUserProfileDialog(props: ChatUserProfileDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [groupDraft, setGroupDraft] = useState<GroupDraft | null>(null)
  const [quotaDialogOpen, setQuotaDialogOpen] = useState(false)
  const [statusAction, setStatusAction] = useState<StatusAction | null>(null)

  const userDetailsQuery = useQuery({
    queryKey: ['messaging', 'user-profile', props.user?.id ?? 0],
    queryFn: () => getUser(props.user?.id ?? 0),
    enabled: props.open && props.canViewDetails && Boolean(props.user?.id),
  })

  const groupsQuery = useQuery({
    queryKey: ['groups'],
    queryFn: getGroups,
    enabled: props.open && props.canViewDetails,
    staleTime: 5 * 60 * 1000,
  })

  const userDetails = userDetailsQuery.data?.data
  const groupValue =
    userDetails && groupDraft?.userId === userDetails.id
      ? groupDraft.group
      : (userDetails?.group ?? '')

  const groupOptions = useMemo(() => {
    const groups = groupsQuery.data?.data ?? []
    if (!groupValue || groups.includes(groupValue)) return groups
    return [groupValue, ...groups]
  }, [groupValue, groupsQuery.data?.data])

  const saveGroupMutation = useMutation({
    mutationFn: async () => {
      if (!userDetails) throw new Error('No user selected')
      return updateUser({
        id: userDetails.id,
        username: userDetails.username,
        display_name: userDetails.display_name,
        role: userDetails.role,
        group: groupValue,
        remark: userDetails.remark || undefined,
      })
    },
    onSuccess: (response) => {
      if (!response.success) {
        toast.error(response.message || t('Failed to update user'))
        return
      }
      toast.success(t('User updated successfully'))
      void invalidateUserDetails(queryClient, props.user?.id)
      props.onUserUpdated()
    },
    onError: () => {
      toast.error(t('Failed to update user'))
    },
  })

  const statusMutation = useMutation({
    mutationFn: async (action: StatusAction) => {
      if (!props.user) throw new Error('No user selected')
      return manageUser(props.user.id, action)
    },
    onSuccess: (response, action) => {
      if (!response.success) {
        toast.error(response.message || t('Failed to update user'))
        return
      }
      const messageKey = action === 'disable' ? 'User disabled' : 'User enabled'
      toast.success(t(messageKey))
      setStatusAction(null)
      void invalidateUserDetails(queryClient, props.user?.id)
      props.onUserUpdated()
      props.onConversationUpdated()
    },
    onError: () => {
      toast.error(t('Failed to update user'))
    },
  })

  const muteMutation = useMutation({
    mutationFn: async (muted: boolean) => {
      if (!props.user || !props.conversation)
        throw new Error('No user selected')
      return setChatMemberMuted(props.conversation.id, props.user.id, { muted })
    },
    onSuccess: (response, muted) => {
      if (!response.success) {
        toast.error(
          response.message || t('Failed to update member mute status')
        )
        return
      }
      toast.success(
        muted ? t('User muted in this group') : t('User unmuted in this group')
      )
      props.onConversationUpdated()
    },
    onError: () => {
      toast.error(t('Failed to update member mute status'))
    },
  })

  if (!props.user) return null

  const chatUser = props.user
  const displayName = getChatUserDisplayName(chatUser)
  const role = userDetails?.role ?? chatUser.role
  const canStartDirectChat = props.currentUserId !== chatUser.id
  const canManageUser = Boolean(
    userDetails &&
    canManageChatProfileUser(props.currentUserRole, userDetails.role)
  )
  const canDisableUser = userDetails?.role !== ROLE.SUPER_ADMIN
  const muteState = getConversationMemberMuteState(
    props.conversation,
    chatUser.id
  )
  const canMuteInGroup = Boolean(
    canManageUser &&
    muteState.isGroupMember &&
    props.currentUserId !== chatUser.id
  )
  const statusActionLabel =
    statusAction === 'disable' ? t('Disable') : t('Enable')
  let muteButtonIcon = <VolumeX className='h-4 w-4' />
  if (muteMutation.isPending) {
    muteButtonIcon = <Loader2 className='h-4 w-4 animate-spin' />
  } else if (muteState.muted) {
    muteButtonIcon = <Volume2 className='h-4 w-4' />
  }

  const handleStartDirectChat = (): void => {
    props.onStartDirectChat(chatUser)
  }

  const handleSaveGroup = (): void => {
    saveGroupMutation.mutate()
  }

  const handleQuotaSuccess = (): void => {
    void invalidateUserDetails(queryClient, props.user?.id)
    props.onUserUpdated()
  }

  const handleToggleMute = (): void => {
    muteMutation.mutate(!muteState.muted)
  }

  const handleConfirmStatusAction = (): void => {
    if (!statusAction) return
    statusMutation.mutate(statusAction)
  }

  return (
    <>
      <Dialog open={props.open} onOpenChange={props.onOpenChange}>
        <DialogContent className='sm:max-w-2xl'>
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
                    {muteState.muted && (
                      <Badge variant='destructive'>{t('Muted in group')}</Badge>
                    )}
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
              <>
                <div className='grid gap-2 rounded-2xl border p-3 sm:grid-cols-2'>
                  <DetailRow
                    label={t('Email')}
                    value={userDetails.email || '-'}
                  />
                  <DetailRow
                    label={t('Group')}
                    value={userDetails.group || '-'}
                  />
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
                      <DetailRow
                        label={t('Remark')}
                        value={userDetails.remark}
                      />
                    </div>
                  )}
                </div>

                {canManageUser ? (
                  <div className='space-y-3 rounded-2xl border p-3'>
                    <div>
                      <div className='text-sm font-semibold'>
                        {t('Admin actions')}
                      </div>
                      <p className='text-muted-foreground mt-1 text-xs'>
                        {t('These operations are only available to admins.')}
                      </p>
                    </div>

                    <div className='grid gap-3 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-end'>
                      <div className='space-y-2'>
                        <Label>{t('Group')}</Label>
                        <Select
                          items={groupOptions.map((group) => ({
                            value: group,
                            label: group,
                          }))}
                          value={groupValue}
                          onValueChange={(value) => {
                            if (value !== null && userDetails) {
                              setGroupDraft({
                                userId: userDetails.id,
                                group: value,
                              })
                            }
                          }}
                        >
                          <SelectTrigger>
                            <SelectValue placeholder={t('Select a group')} />
                          </SelectTrigger>
                          <SelectContent alignItemWithTrigger={false}>
                            <SelectGroup>
                              {groupOptions.map((group) => (
                                <SelectItem key={group} value={group}>
                                  {group}
                                </SelectItem>
                              ))}
                            </SelectGroup>
                          </SelectContent>
                        </Select>
                      </div>
                      <Button
                        type='button'
                        onClick={handleSaveGroup}
                        disabled={
                          saveGroupMutation.isPending ||
                          !groupValue ||
                          groupValue === userDetails.group
                        }
                        className='gap-2'
                      >
                        {saveGroupMutation.isPending ? (
                          <Loader2 className='h-4 w-4 animate-spin' />
                        ) : (
                          <Save className='h-4 w-4' />
                        )}
                        {t('Save group')}
                      </Button>
                    </div>

                    <div className='flex flex-wrap gap-2'>
                      <Button
                        type='button'
                        variant='outline'
                        onClick={() => setQuotaDialogOpen(true)}
                        className='gap-2'
                      >
                        <Power className='h-4 w-4' />
                        {t('Adjust Quota')}
                      </Button>
                      {userDetails.status === USER_STATUS.DISABLED ? (
                        <Button
                          type='button'
                          variant='outline'
                          onClick={() => setStatusAction('enable')}
                          className='gap-2'
                        >
                          <Power className='h-4 w-4' />
                          {t('Enable user')}
                        </Button>
                      ) : (
                        <Button
                          type='button'
                          variant='destructive'
                          onClick={() => setStatusAction('disable')}
                          disabled={!canDisableUser}
                          className='gap-2'
                        >
                          <PowerOff className='h-4 w-4' />
                          {t('Disable user')}
                        </Button>
                      )}
                      {muteState.isGroupMember && (
                        <Button
                          type='button'
                          variant='outline'
                          onClick={handleToggleMute}
                          disabled={!canMuteInGroup || muteMutation.isPending}
                          className='gap-2'
                        >
                          {muteButtonIcon}
                          {muteState.muted
                            ? t('Unmute in group')
                            : t('Mute in group')}
                        </Button>
                      )}
                    </div>
                  </div>
                ) : (
                  <Alert>
                    <Ban className='h-4 w-4' />
                    <AlertDescription>
                      {t(
                        'You cannot manage users with the same or higher role.'
                      )}
                    </AlertDescription>
                  </Alert>
                )}
              </>
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

      {userDetails && (
        <UserQuotaDialog
          open={quotaDialogOpen}
          onOpenChange={setQuotaDialogOpen}
          userId={userDetails.id}
          currentQuota={userDetails.quota}
          onSuccess={handleQuotaSuccess}
        />
      )}

      <ConfirmDialog
        open={statusAction !== null}
        onOpenChange={(open) => {
          if (!open) setStatusAction(null)
        }}
        title={statusActionLabel}
        desc={t('Are you sure you want to {{action}} this user?', {
          action: statusActionLabel,
        })}
        confirmText={statusActionLabel}
        destructive={statusAction === 'disable'}
        isLoading={statusMutation.isPending}
        handleConfirm={handleConfirmStatusAction}
      />
    </>
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

function invalidateUserDetails(
  queryClient: ReturnType<typeof useQueryClient>,
  userId?: number
): Promise<unknown> {
  if (!userId) return Promise.resolve()
  return queryClient.invalidateQueries({
    queryKey: ['messaging', 'user-profile', userId],
  })
}
