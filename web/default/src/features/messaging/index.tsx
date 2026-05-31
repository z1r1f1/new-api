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
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Loader2,
  MessageCircle,
  MessageSquarePlus,
  Plus,
  Send,
  UserPlus,
  Users,
} from 'lucide-react'
import { nanoid } from 'nanoid'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { useAuthStore } from '@/stores/auth-store'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Textarea } from '@/components/ui/textarea'
import {
  addChatMember,
  createDirectConversation,
  createGroupConversation,
  listChatConversations,
  listChatMessages,
  markChatRead,
  messagingQueryKeys,
  sendChatMessage,
} from './api'
import { useChatRealtime } from './hooks/use-chat-realtime'
import type { ChatConversation, ChatEvent, ChatMessage } from './types'

function parseMemberIds(value: string): number[] {
  return value
    .split(',')
    .map((item) => Number(item.trim()))
    .filter((item) => Number.isInteger(item) && item > 0)
}

function formatChatTime(timestamp: number): string {
  if (!timestamp) return ''
  return new Intl.DateTimeFormat(undefined, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(timestamp * 1000))
}

function getConversationTitle(conversation: ChatConversation): string {
  if (conversation.title.trim()) return conversation.title
  return ''
}

export function Messaging() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const currentUserId = useAuthStore((state) => state.auth.user?.id ?? null)
  const [activeConversationIdState, setActiveConversationId] = useState<
    number | null
  >(null)
  const [messageBody, setMessageBody] = useState('')
  const [directUserId, setDirectUserId] = useState('')
  const [groupTitle, setGroupTitle] = useState('')
  const [groupMemberIds, setGroupMemberIds] = useState('')
  const [memberUserId, setMemberUserId] = useState('')

  const conversationsQuery = useQuery({
    queryKey: messagingQueryKeys.conversations,
    queryFn: listChatConversations,
    select: (response) => response.data ?? [],
  })

  const conversations = useMemo(
    () => conversationsQuery.data ?? [],
    [conversationsQuery.data]
  )
  const activeConversationId =
    activeConversationIdState ?? conversations[0]?.id ?? null
  const activeConversation = useMemo(
    () =>
      conversations.find(
        (conversation) => conversation.id === activeConversationId
      ) ?? null,
    [activeConversationId, conversations]
  )

  const messagesQuery = useQuery({
    queryKey: activeConversationId
      ? messagingQueryKeys.messages(activeConversationId)
      : ['messaging', 'messages', 'idle'],
    queryFn: () => listChatMessages(activeConversationId ?? 0),
    enabled: Boolean(activeConversationId),
    select: (response) => response.data ?? [],
  })

  const messages = useMemo(() => messagesQuery.data ?? [], [messagesQuery.data])

  const invalidateConversations = useCallback(() => {
    void queryClient.invalidateQueries({
      queryKey: messagingQueryKeys.conversations,
    })
  }, [queryClient])

  const appendMessage = useCallback(
    (message: ChatMessage) => {
      queryClient.setQueryData(
        messagingQueryKeys.messages(message.conversation_id),
        (current: { data?: ChatMessage[]; success?: boolean } | undefined) => {
          const existing = current?.data ?? []
          if (existing.some((item) => item.id === message.id)) return current
          return {
            success: true,
            data: [...existing, message],
          }
        }
      )
    },
    [queryClient]
  )

  const handleRealtimeEvent = useCallback(
    (event: ChatEvent) => {
      if (event.type === 'conversation.created') {
        invalidateConversations()
        return
      }
      if (event.type === 'conversation.updated') {
        invalidateConversations()
        return
      }
      if (event.type === 'message.created' && event.message) {
        appendMessage(event.message)
        invalidateConversations()
      }
    },
    [appendMessage, invalidateConversations]
  )

  const realtimeStatus = useChatRealtime({
    userId: currentUserId,
    conversationId: activeConversationId,
    onEvent: handleRealtimeEvent,
  })

  const sendMutation = useMutation({
    mutationFn: () => {
      if (!activeConversationId)
        throw new Error(t('Select a conversation first'))
      return sendChatMessage(activeConversationId, {
        body: messageBody.trim(),
        client_message_id: nanoid(),
      })
    },
    onSuccess: (response) => {
      if (response.data) appendMessage(response.data)
      setMessageBody('')
      invalidateConversations()
    },
  })

  const createDirectMutation = useMutation({
    mutationFn: () =>
      createDirectConversation({ user_id: Number(directUserId.trim()) }),
    onSuccess: (response) => {
      if (response.data) setActiveConversationId(response.data.id)
      setDirectUserId('')
      invalidateConversations()
      toast.success(t('Conversation ready'))
    },
  })

  const createGroupMutation = useMutation({
    mutationFn: () =>
      createGroupConversation({
        title: groupTitle.trim(),
        member_ids: parseMemberIds(groupMemberIds),
      }),
    onSuccess: (response) => {
      if (response.data) setActiveConversationId(response.data.id)
      setGroupTitle('')
      setGroupMemberIds('')
      invalidateConversations()
      toast.success(t('Group conversation created'))
    },
  })

  const addMemberMutation = useMutation({
    mutationFn: () => {
      if (!activeConversationId)
        throw new Error(t('Select a conversation first'))
      return addChatMember(activeConversationId, {
        user_id: Number(memberUserId.trim()),
      })
    },
    onSuccess: () => {
      setMemberUserId('')
      toast.success(t('Member added'))
    },
  })

  useEffect(() => {
    const lastMessage = messages.at(-1)
    if (!activeConversationId || !lastMessage) return
    void markChatRead(activeConversationId, {
      last_read_message_id: lastMessage.id,
    })
  }, [activeConversationId, messages])

  const handleSendMessage = (): void => {
    if (!messageBody.trim()) return
    sendMutation.mutate()
  }

  const handleCreateDirect = (): void => {
    const userId = Number(directUserId.trim())
    if (!Number.isInteger(userId) || userId <= 0) {
      toast.error(t('Enter a valid user ID'))
      return
    }
    createDirectMutation.mutate()
  }

  const handleCreateGroup = (): void => {
    if (!groupTitle.trim()) {
      toast.error(t('Enter a group name'))
      return
    }
    createGroupMutation.mutate()
  }

  const handleAddMember = (): void => {
    const userId = Number(memberUserId.trim())
    if (!Number.isInteger(userId) || userId <= 0) {
      toast.error(t('Enter a valid user ID'))
      return
    }
    addMemberMutation.mutate()
  }

  return (
    <div className='bg-background flex h-full min-h-0 flex-col gap-4 p-4 lg:flex-row'>
      <aside className='flex min-h-0 w-full flex-col gap-4 lg:w-88'>
        <Card className='min-h-0 flex-1'>
          <CardHeader className='pb-3'>
            <div className='flex items-center justify-between gap-3'>
              <CardTitle className='flex items-center gap-2 text-base'>
                <MessageCircle className='h-4 w-4' />
                {t('Messages')}
              </CardTitle>
              <Badge
                variant={realtimeStatus === 'connected' ? 'default' : 'outline'}
              >
                {t(realtimeStatus)}
              </Badge>
            </div>
          </CardHeader>
          <CardContent className='min-h-0 p-0'>
            <ConversationList
              conversations={conversations}
              activeConversationId={activeConversationId}
              loading={conversationsQuery.isPending}
              onSelect={setActiveConversationId}
            />
          </CardContent>
        </Card>
        <Card>
          <CardHeader className='pb-3'>
            <CardTitle className='flex items-center gap-2 text-sm'>
              <MessageSquarePlus className='h-4 w-4' />
              {t('Start conversation')}
            </CardTitle>
          </CardHeader>
          <CardContent className='space-y-3'>
            <div className='flex gap-2'>
              <Input
                value={directUserId}
                inputMode='numeric'
                onChange={(event) => setDirectUserId(event.target.value)}
                placeholder={t('Peer user ID')}
              />
              <Button
                type='button'
                onClick={handleCreateDirect}
                disabled={createDirectMutation.isPending}
              >
                <Plus className='h-4 w-4' />
              </Button>
            </div>
            <div className='space-y-2'>
              <Input
                value={groupTitle}
                onChange={(event) => setGroupTitle(event.target.value)}
                placeholder={t('Group name')}
              />
              <div className='flex gap-2'>
                <Input
                  value={groupMemberIds}
                  onChange={(event) => setGroupMemberIds(event.target.value)}
                  placeholder={t('Member IDs, separated by commas')}
                />
                <Button
                  type='button'
                  variant='secondary'
                  onClick={handleCreateGroup}
                  disabled={createGroupMutation.isPending}
                >
                  <Users className='h-4 w-4' />
                </Button>
              </div>
            </div>
          </CardContent>
        </Card>
      </aside>

      <main className='min-w-0 flex-1'>
        <Card className='flex h-full min-h-0 flex-col'>
          <CardHeader className='border-border border-b'>
            <div className='flex flex-col gap-3 md:flex-row md:items-center md:justify-between'>
              <div>
                <CardTitle className='text-base'>
                  {activeConversation ? (
                    <ConversationTitle conversation={activeConversation} />
                  ) : (
                    t('Select a conversation')
                  )}
                </CardTitle>
                {activeConversation && (
                  <p className='text-muted-foreground mt-1 text-xs'>
                    {t('Conversation ID')}: {activeConversation.id} ·{' '}
                    {t(activeConversation.type)}
                  </p>
                )}
              </div>
              {activeConversation?.type === 'group' && (
                <div className='flex gap-2'>
                  <Input
                    value={memberUserId}
                    inputMode='numeric'
                    onChange={(event) => setMemberUserId(event.target.value)}
                    placeholder={t('User ID')}
                    className='w-36'
                  />
                  <Button
                    type='button'
                    variant='outline'
                    onClick={handleAddMember}
                    disabled={addMemberMutation.isPending}
                  >
                    <UserPlus className='h-4 w-4' />
                    {t('Add')}
                  </Button>
                </div>
              )}
            </div>
          </CardHeader>
          <CardContent className='flex min-h-0 flex-1 flex-col p-0'>
            <MessageList
              messages={messages}
              currentUserId={currentUserId}
              loading={messagesQuery.isPending}
              empty={!activeConversation}
            />
            <div className='border-border flex gap-2 border-t p-3'>
              <Textarea
                value={messageBody}
                onChange={(event) => setMessageBody(event.target.value)}
                placeholder={t('Type a message...')}
                disabled={!activeConversation || sendMutation.isPending}
                className='min-h-12 resize-none'
                onKeyDown={(event) => {
                  if (event.key === 'Enter' && !event.shiftKey) {
                    event.preventDefault()
                    handleSendMessage()
                  }
                }}
              />
              <Button
                type='button'
                onClick={handleSendMessage}
                disabled={!activeConversation || sendMutation.isPending}
                className='self-end'
              >
                <Send className='h-4 w-4' />
                {t('Send')}
              </Button>
            </div>
          </CardContent>
        </Card>
      </main>
    </div>
  )
}

interface ConversationListProps {
  conversations: ChatConversation[]
  activeConversationId: number | null
  loading: boolean
  onSelect: (conversationId: number) => void
}

function ConversationList(props: ConversationListProps) {
  const { t } = useTranslation()

  if (props.loading) {
    return (
      <div className='text-muted-foreground flex items-center gap-2 p-4 text-sm'>
        <Loader2 className='h-4 w-4 animate-spin' />
        {t('Loading conversations...')}
      </div>
    )
  }

  if (props.conversations.length === 0) {
    return (
      <div className='text-muted-foreground p-4 text-sm'>
        {t('No conversations yet. Start one with a user ID.')}
      </div>
    )
  }

  return (
    <ScrollArea className='h-[360px] lg:h-[calc(100vh-20rem)]'>
      <div className='space-y-1 p-2'>
        {props.conversations.map((conversation) => (
          <button
            key={conversation.id}
            type='button'
            onClick={() => props.onSelect(conversation.id)}
            className={cn(
              'hover:bg-muted w-full rounded-lg px-3 py-2 text-left transition-colors',
              props.activeConversationId === conversation.id && 'bg-muted'
            )}
          >
            <div className='flex items-center justify-between gap-2'>
              <span className='truncate text-sm font-medium'>
                <ConversationTitle conversation={conversation} />
              </span>
              <Badge variant='outline'>{t(conversation.type)}</Badge>
            </div>
            <div className='text-muted-foreground mt-1 flex justify-between gap-2 text-xs'>
              <span>
                {t('Last message')}: #{conversation.last_message_id || '-'}
              </span>
              <span>{formatChatTime(conversation.last_message_at)}</span>
            </div>
          </button>
        ))}
      </div>
    </ScrollArea>
  )
}

interface ConversationTitleProps {
  conversation: ChatConversation
}

function ConversationTitle(props: ConversationTitleProps) {
  const { t } = useTranslation()
  const explicitTitle = getConversationTitle(props.conversation)
  if (explicitTitle) return explicitTitle
  if (props.conversation.type === 'direct') {
    return (
      <>
        {t('Direct chat')} #{props.conversation.id}
      </>
    )
  }
  return (
    <>
      {t('Group chat')} #{props.conversation.id}
    </>
  )
}

interface MessageListProps {
  messages: ChatMessage[]
  currentUserId: number | null
  loading: boolean
  empty: boolean
}

function MessageList(props: MessageListProps) {
  const { t } = useTranslation()

  if (props.empty) {
    return (
      <div className='text-muted-foreground flex flex-1 items-center justify-center p-6 text-center text-sm'>
        {t('Select a conversation to read messages.')}
      </div>
    )
  }

  if (props.loading) {
    return (
      <div className='text-muted-foreground flex flex-1 items-center justify-center gap-2 p-6 text-sm'>
        <Loader2 className='h-4 w-4 animate-spin' />
        {t('Loading messages...')}
      </div>
    )
  }

  if (props.messages.length === 0) {
    return (
      <div className='text-muted-foreground flex flex-1 items-center justify-center p-6 text-center text-sm'>
        {t('No messages yet. Send the first message below.')}
      </div>
    )
  }

  return (
    <ScrollArea className='min-h-0 flex-1'>
      <div className='space-y-3 p-4'>
        {props.messages.map((message) => {
          const isMine = message.sender_id === props.currentUserId
          return (
            <div
              key={message.id}
              className={cn('flex', isMine ? 'justify-end' : 'justify-start')}
            >
              <div
                className={cn(
                  'max-w-[80%] rounded-2xl px-3 py-2 text-sm shadow-sm',
                  isMine
                    ? 'bg-primary text-primary-foreground'
                    : 'bg-muted text-foreground'
                )}
              >
                <div className='mb-1 text-xs opacity-75'>
                  {t('User')} #{message.sender_id} ·{' '}
                  {formatChatTime(message.created_at)}
                </div>
                <div className='break-words whitespace-pre-wrap'>
                  {message.body}
                </div>
              </div>
            </div>
          )
        })}
      </div>
    </ScrollArea>
  )
}
