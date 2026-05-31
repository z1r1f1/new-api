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
import {
  Bell,
  Clock3,
  Crown,
  Filter,
  Hash,
  Info,
  Loader2,
  MessageCircle,
  MoreHorizontal,
  ShieldCheck,
  UserMinus,
  UserPlus,
  Users,
  type LucideIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import {
  Avatar,
  AvatarFallback,
  AvatarGroup,
  AvatarGroupCount,
} from '@/components/ui/avatar'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { formatChatTime } from '../lib/format'
import type { ChatConversation } from '../types'
import { ConversationAvatar } from './conversation-avatar'
import { ConversationTitle } from './conversation-title'
import { RealtimeStatusBadge } from './realtime-status-badge'

interface ConversationDetailsPanelProps {
  conversation: ChatConversation | null
  currentUserId: number | null
  realtimeStatus: string
  messageCount: number
  memberUserId: string
  removeMemberUserId: string
  addingMember: boolean
  removingMember: boolean
  onMemberUserIdChange: (value: string) => void
  onRemoveMemberUserIdChange: (value: string) => void
  onAddMember: () => void
  onRemoveMember: () => void
}

export function ConversationDetailsPanel(props: ConversationDetailsPanelProps) {
  const { t } = useTranslation()

  return (
    <Card className='border-border/80 bg-background/95 flex h-full min-h-0 w-full flex-col overflow-hidden shadow-sm'>
      <CardHeader className='border-border/80 border-b pb-4'>
        <CardTitle className='flex items-center gap-2 text-base'>
          <Info className='h-4 w-4' />
          {t('Conversation details')}
        </CardTitle>
      </CardHeader>
      <CardContent className='min-h-0 flex-1 overflow-y-auto p-4'>
        {props.conversation ? (
          <ConversationDetailsContent
            {...props}
            conversation={props.conversation}
          />
        ) : (
          <NoConversationDetails />
        )}
      </CardContent>
    </Card>
  )
}

function ConversationDetailsContent(
  props: ConversationDetailsPanelProps & { conversation: ChatConversation }
) {
  const { t } = useTranslation()

  return (
    <div className='space-y-5'>
      <div className='bg-muted/20 rounded-3xl border p-4 text-center'>
        <ConversationAvatar
          conversation={props.conversation}
          className='mx-auto mb-3 size-14'
        />
        <h2 className='truncate text-base font-semibold'>
          <ConversationTitle conversation={props.conversation} />
        </h2>
        <p className='text-muted-foreground mt-1 text-xs'>
          {t('Workspace #{{id}}', { id: props.conversation.id })}
        </p>
        <div className='mt-3 flex justify-center gap-2'>
          <Badge variant='outline' className='rounded-full'>
            {t(props.conversation.type)}
          </Badge>
          <RealtimeStatusBadge status={props.realtimeStatus} />
        </div>
      </div>

      <div className='grid grid-cols-2 gap-2'>
        <DetailStat
          icon={MessageCircle}
          label={t('Messages')}
          value={props.messageCount}
        />
        <DetailStat
          icon={Clock3}
          label={t('Last active')}
          value={formatChatTime(props.conversation.last_message_at)}
        />
        <DetailStat
          icon={Crown}
          label={t('Owner')}
          value={`#${props.conversation.owner_id || '-'}`}
        />
        <DetailStat
          icon={Hash}
          label={t('Current user')}
          value={`#${props.currentUserId ?? '-'}`}
        />
      </div>

      <MemberOperationsCard {...props} />
      <EnterpriseSafeguardsCard />
      <ParticipantSnapshot
        conversation={props.conversation}
        currentUserId={props.currentUserId}
      />
    </div>
  )
}

function MemberOperationsCard(
  props: ConversationDetailsPanelProps & { conversation: ChatConversation }
) {
  const { t } = useTranslation()

  return (
    <div className='bg-muted/20 rounded-3xl border p-4'>
      <div className='mb-3 flex items-center gap-2 text-sm font-medium'>
        <Users className='h-4 w-4' />
        {t('Member operations')}
      </div>
      {props.conversation.type === 'group' ? (
        <div className='space-y-3'>
          <div className='flex gap-2'>
            <Input
              value={props.memberUserId}
              inputMode='numeric'
              onChange={(event) =>
                props.onMemberUserIdChange(event.target.value)
              }
              placeholder={t('User ID')}
            />
            <Button
              type='button'
              variant='outline'
              onClick={props.onAddMember}
              disabled={props.addingMember}
            >
              {props.addingMember ? (
                <Loader2 className='h-4 w-4 animate-spin' />
              ) : (
                <UserPlus className='h-4 w-4' />
              )}
            </Button>
          </div>
          <div className='flex gap-2'>
            <Input
              value={props.removeMemberUserId}
              inputMode='numeric'
              onChange={(event) =>
                props.onRemoveMemberUserIdChange(event.target.value)
              }
              placeholder={t('Remove user ID')}
            />
            <Button
              type='button'
              variant='destructive'
              onClick={props.onRemoveMember}
              disabled={props.removingMember}
            >
              {props.removingMember ? (
                <Loader2 className='h-4 w-4 animate-spin' />
              ) : (
                <UserMinus className='h-4 w-4' />
              )}
            </Button>
          </div>
          <p className='text-muted-foreground text-xs'>
            {t(
              'Only group owners can add or remove members. Changes are protected by the backend permission checks.'
            )}
          </p>
        </div>
      ) : (
        <p className='text-muted-foreground text-sm'>
          {t(
            'Direct conversations are limited to two users. Create a group workspace when more people need to join.'
          )}
        </p>
      )}
    </div>
  )
}

function EnterpriseSafeguardsCard() {
  const { t } = useTranslation()

  return (
    <div className='bg-muted/20 rounded-3xl border p-4'>
      <div className='mb-3 flex items-center gap-2 text-sm font-medium'>
        <ShieldCheck className='h-4 w-4 text-emerald-500' />
        {t('Enterprise safeguards')}
      </div>
      <div className='space-y-3 text-sm'>
        <SafeguardRow
          icon={ShieldCheck}
          title={t('Authenticated API')}
          description={t(
            'All REST operations use the existing user session and permission middleware.'
          )}
        />
        <SafeguardRow
          icon={Bell}
          title={t('Realtime delivery')}
          description={t(
            'Live updates arrive through the dedicated WebSocket channel.'
          )}
        />
        <SafeguardRow
          icon={Filter}
          title={t('Operational visibility')}
          description={t(
            'Search, filters, IDs, and timestamps keep support handoffs traceable.'
          )}
        />
      </div>
    </div>
  )
}

interface ParticipantSnapshotProps {
  conversation: ChatConversation
  currentUserId: number | null
}

function ParticipantSnapshot(props: ParticipantSnapshotProps) {
  const { t } = useTranslation()

  return (
    <div className='bg-muted/20 rounded-3xl border p-4'>
      <div className='mb-3 text-sm font-medium'>
        {t('Participant snapshot')}
      </div>
      <AvatarGroup className='justify-start'>
        <Avatar size='sm'>
          <AvatarFallback>#{props.currentUserId ?? '-'}</AvatarFallback>
        </Avatar>
        <Avatar size='sm'>
          <AvatarFallback>#{props.conversation.owner_id || '-'}</AvatarFallback>
        </Avatar>
        <AvatarGroupCount>
          <MoreHorizontal className='h-4 w-4' />
        </AvatarGroupCount>
      </AvatarGroup>
      <p className='text-muted-foreground mt-3 text-xs'>
        {t(
          'Detailed member lists can be added when the backend exposes a members endpoint.'
        )}
      </p>
    </div>
  )
}

function NoConversationDetails() {
  const { t } = useTranslation()

  return (
    <div className='flex h-full items-center justify-center text-center'>
      <div>
        <div className='bg-muted mx-auto mb-3 flex size-12 items-center justify-center rounded-2xl'>
          <Info className='text-muted-foreground h-5 w-5' />
        </div>
        <p className='text-sm font-medium'>{t('No conversation selected')}</p>
        <p className='text-muted-foreground mt-1 text-xs'>
          {t(
            'Select a chat to inspect metadata, safeguards, and member actions.'
          )}
        </p>
      </div>
    </div>
  )
}

interface DetailStatProps {
  icon: LucideIcon
  label: string
  value: string | number
}

function DetailStat(props: DetailStatProps) {
  const Icon = props.icon
  return (
    <div className='bg-card rounded-2xl border p-3'>
      <Icon className='text-muted-foreground mb-2 h-4 w-4' />
      <div className='text-muted-foreground text-[11px]'>{props.label}</div>
      <div className='mt-1 truncate text-sm font-semibold'>{props.value}</div>
    </div>
  )
}

interface SafeguardRowProps {
  icon: LucideIcon
  title: string
  description: string
}

function SafeguardRow(props: SafeguardRowProps) {
  const Icon = props.icon
  return (
    <div className='flex gap-3'>
      <div className='bg-background mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-xl'>
        <Icon className='h-4 w-4' />
      </div>
      <div className='min-w-0'>
        <div className='font-medium'>{props.title}</div>
        <div className='text-muted-foreground text-xs leading-relaxed'>
          {props.description}
        </div>
      </div>
    </div>
  )
}
