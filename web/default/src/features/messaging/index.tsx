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
import { Loader2, MessageCircle, RotateCcw, Search } from 'lucide-react'
import { nanoid } from 'nanoid'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { useAuthStore } from '@/stores/auth-store'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import {
  createDirectConversation,
  listChatConversations,
  listChatMessages,
  listChatUsers,
  markChatRead,
  messagingQueryKeys,
  sendChatMessage,
} from './api'
import { MessageComposer } from './components/message-composer'
import { MessageList } from './components/message-list'
import { RealtimeStatusBadge } from './components/realtime-status-badge'
import { UserSidebar } from './components/user-sidebar'
import { useChatRealtime } from './hooks/use-chat-realtime'
import {
  filterChatUsers,
  filterMessages,
  getChatUserDisplayName,
  getChatUserInitial,
  getDirectConversationKey,
  MAX_MESSAGE_LENGTH,
  mergeMessages,
  MESSAGE_PAGE_SIZE,
} from './lib/format'
import type {
  ApiResponse,
  ChatConversation,
  ChatEvent,
  ChatMessage,
  ChatUser,
} from './types'

export function Messaging() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const currentUserId = useAuthStore((state) => state.auth.user?.id ?? null)
  const [selectedUserId, setSelectedUserId] = useState<number | null>(null)
  const [activeConversationId, setActiveConversationId] = useState<
    number | null
  >(null)
  const [openingUserId, setOpeningUserId] = useState<number | null>(null)
  const [messageBody, setMessageBody] = useState('')
  const [userSearch, setUserSearch] = useState('')
  const [messageSearch, setMessageSearch] = useState('')
  const [historyExhaustedByConversation, setHistoryExhaustedByConversation] =
    useState<Record<number, boolean>>({})

  const usersQuery = useQuery({
    queryKey: messagingQueryKeys.users,
    queryFn: listChatUsers,
    select: (response) => response.data ?? [],
  })

  const conversationsQuery = useQuery({
    queryKey: messagingQueryKeys.conversations,
    queryFn: listChatConversations,
    select: (response) => response.data ?? [],
  })

  const users = useMemo(() => usersQuery.data ?? [], [usersQuery.data])
  const filteredUsers = useMemo(
    () => filterChatUsers(users, userSearch),
    [userSearch, users]
  )
  const conversations = useMemo(
    () => conversationsQuery.data ?? [],
    [conversationsQuery.data]
  )
  const selectedUser = useMemo(
    () => users.find((user) => user.id === selectedUserId) ?? null,
    [selectedUserId, users]
  )
  const activeConversation = useMemo(
    () =>
      activeConversationId
        ? conversations.find(
            (conversation) => conversation.id === activeConversationId
          ) ?? null
        : null,
    [activeConversationId, conversations]
  )

  const messagesQuery = useQuery({
    queryKey: activeConversationId
      ? messagingQueryKeys.messages(activeConversationId)
      : ['messaging', 'messages', 'idle'],
    queryFn: () =>
      listChatMessages(activeConversationId ?? 0, 0, MESSAGE_PAGE_SIZE),
    enabled: Boolean(activeConversationId),
    select: (response) => response.data ?? [],
  })

  const messages = useMemo(() => messagesQuery.data ?? [], [messagesQuery.data])
  const filteredMessages = useMemo(
    () => filterMessages(messages, messageSearch),
    [messageSearch, messages]
  )

  const invalidateUsers = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: messagingQueryKeys.users })
  }, [queryClient])

  const invalidateConversations = useCallback(() => {
    void queryClient.invalidateQueries({
      queryKey: messagingQueryKeys.conversations,
    })
  }, [queryClient])

  const appendMessage = useCallback(
    (message: ChatMessage) => {
      queryClient.setQueryData(
        messagingQueryKeys.messages(message.conversation_id),
        (current: ApiResponse<ChatMessage[]> | undefined) => ({
          success: true,
          data: mergeMessages(current?.data ?? [], [message]),
        })
      )
    },
    [queryClient]
  )

  const openDirectConversation = useCallback(
    (user: ChatUser, knownConversations: ChatConversation[]): boolean => {
      const directKey = getDirectConversationKey(currentUserId, user.id)
      const existingConversation = knownConversations.find(
        (conversation) =>
          conversation.type === 'direct' && conversation.direct_key === directKey
      )
      if (!existingConversation) return false
      setActiveConversationId(existingConversation.id)
      return true
    },
    [currentUserId]
  )

  const createDirectMutation = useMutation({
    mutationFn: (peerUserId: number) =>
      createDirectConversation({ user_id: peerUserId }),
    onMutate: (peerUserId) => {
      setOpeningUserId(peerUserId)
    },
    onSuccess: (response) => {
      if (response.data) {
        setActiveConversationId(response.data.id)
        void queryClient.invalidateQueries({
          queryKey: messagingQueryKeys.messages(response.data.id),
        })
      }
      invalidateConversations()
    },
    onError: () => {
      toast.error(t('Failed to open conversation'))
    },
    onSettled: () => {
      setOpeningUserId(null)
    },
  })

  const sendMutation = useMutation({
    mutationFn: () => {
      if (!activeConversationId) {
        throw new Error(t('Select a user first'))
      }
      return sendChatMessage(activeConversationId, {
        body: messageBody.trim(),
        client_message_id: nanoid(),
      })
    },
    onSuccess: (response) => {
      if (response.data) appendMessage(response.data)
      setMessageBody('')
      setMessageSearch('')
      invalidateConversations()
    },
    onError: () => {
      toast.error(t('Failed to send message'))
    },
  })

  const loadOlderMutation = useMutation({
    mutationFn: () => {
      if (!activeConversationId || messages.length === 0) {
        throw new Error(t('No earlier messages to load'))
      }
      return listChatMessages(
        activeConversationId,
        messages[0].id,
        MESSAGE_PAGE_SIZE
      )
    },
    onSuccess: (response) => {
      if (!activeConversationId) return
      const olderMessages = response.data ?? []
      queryClient.setQueryData(
        messagingQueryKeys.messages(activeConversationId),
        (current: ApiResponse<ChatMessage[]> | undefined) => ({
          success: true,
          data: mergeMessages(current?.data ?? [], olderMessages),
        })
      )
      if (olderMessages.length < MESSAGE_PAGE_SIZE) {
        setHistoryExhaustedByConversation((current) => ({
          ...current,
          [activeConversationId]: true,
        }))
      }
    },
  })

  const handleRealtimeEvent = useCallback(
    (event: ChatEvent) => {
      if (event.type === 'message.created' && event.message) {
        appendMessage(event.message)
        invalidateConversations()
        return
      }
      if (
        event.type === 'conversation.created' ||
        event.type === 'conversation.updated'
      ) {
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

  useEffect(() => {
    const lastMessage = messages.at(-1)
    if (!activeConversationId || !lastMessage) return
    void markChatRead(activeConversationId, {
      last_read_message_id: lastMessage.id,
    })
  }, [activeConversationId, messages])

  const handleSelectUser = (user: ChatUser): void => {
    setSelectedUserId(user.id)
    setMessageBody('')
    setMessageSearch('')
    setActiveConversationId(null)
    if (openDirectConversation(user, conversations)) return
    createDirectMutation.mutate(user.id)
  }

  const handleSendMessage = (): void => {
    const body = messageBody.trim()
    if (!body) return
    if (body.length > MAX_MESSAGE_LENGTH) {
      toast.error(t('Message is too long'))
      return
    }
    sendMutation.mutate()
  }

  const handleRefresh = (): void => {
    invalidateUsers()
    invalidateConversations()
    if (activeConversationId) {
      void queryClient.invalidateQueries({
        queryKey: messagingQueryKeys.messages(activeConversationId),
      })
    }
  }

  const canLoadOlder = Boolean(
    activeConversationId &&
      messages.length > 0 &&
      !historyExhaustedByConversation[activeConversationId]
  )
  const loadingThread = Boolean(
    selectedUser && !activeConversation && createDirectMutation.isPending
  )

  return (
    <div className='bg-background flex h-full min-h-0 p-3 lg:p-4'>
      <div className='grid min-h-0 flex-1 gap-4 lg:grid-cols-[20rem_minmax(0,1fr)]'>
        <aside className='min-h-0'>
          <UserSidebar
            users={filteredUsers}
            activeUserId={selectedUserId}
            loading={usersQuery.isPending}
            searchText={userSearch}
            openingUserId={openingUserId}
            onSearchChange={setUserSearch}
            onSelect={handleSelectUser}
            onRefresh={handleRefresh}
          />
        </aside>

        <main className='min-w-0'>
          <Card className='border-border/80 flex h-full min-h-0 overflow-hidden shadow-sm'>
            <div className='flex min-w-0 flex-1 flex-col'>
              <ChatHeader
                user={selectedUser}
                realtimeStatus={realtimeStatus}
                messageCount={messages.length}
                searchText={messageSearch}
                loading={loadingThread}
                onSearchChange={setMessageSearch}
                onRefresh={handleRefresh}
              />
              <CardContent className='flex min-h-0 flex-1 flex-col p-0'>
                <MessageList
                  messages={filteredMessages}
                  allMessageCount={messages.length}
                  currentUserId={currentUserId}
                  loading={messagesQuery.isPending || loadingThread}
                  empty={!selectedUser}
                  searchText={messageSearch}
                  canLoadOlder={canLoadOlder}
                  loadingOlder={loadOlderMutation.isPending}
                  onLoadOlder={() => loadOlderMutation.mutate()}
                />
                <MessageComposer
                  value={messageBody}
                  active={Boolean(selectedUser && activeConversationId)}
                  sending={sendMutation.isPending}
                  maxLength={MAX_MESSAGE_LENGTH}
                  onChange={setMessageBody}
                  onSend={handleSendMessage}
                />
              </CardContent>
            </div>
          </Card>
        </main>
      </div>
    </div>
  )
}

interface ChatHeaderProps {
  user: ChatUser | null
  realtimeStatus: string
  messageCount: number
  searchText: string
  loading: boolean
  onSearchChange: (value: string) => void
  onRefresh: () => void
}

function ChatHeader(props: ChatHeaderProps) {
  const { t } = useTranslation()
  const title = props.user
    ? t('Chat with {{name}}', { name: `@${props.user.username}` })
    : t('Select a user')

  return (
    <CardHeader className='border-border/80 border-b'>
      <div className='flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between'>
        <div className='flex min-w-0 items-center gap-3'>
          {props.user ? (
            <Avatar size='lg' className='shadow-sm'>
              <AvatarFallback className='bg-muted'>
                {getChatUserInitial(props.user)}
              </AvatarFallback>
            </Avatar>
          ) : (
            <div className='bg-muted flex size-10 items-center justify-center rounded-full'>
              <MessageCircle className='text-muted-foreground h-5 w-5' />
            </div>
          )}
          <div className='min-w-0'>
            <CardTitle className='truncate text-base'>{title}</CardTitle>
            <div className='text-muted-foreground mt-1 flex items-center gap-2 text-xs'>
              {props.user ? (
                <>
                  <span>{getChatUserDisplayName(props.user)}</span>
                  <span>·</span>
                  <span>
                    {t('{{count}} messages', { count: props.messageCount })}
                  </span>
                  {props.loading && (
                    <>
                      <span>·</span>
                      <Loader2 className='h-3 w-3 animate-spin' />
                    </>
                  )}
                </>
              ) : (
                <span>{t('Choose a user from the left to start chatting.')}</span>
              )}
            </div>
          </div>
        </div>
        <div className='flex flex-col gap-2 sm:flex-row sm:items-center'>
          <div className='relative min-w-0 sm:w-64'>
            <Search className='text-muted-foreground absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2' />
            <Input
              value={props.searchText}
              onChange={(event) => props.onSearchChange(event.target.value)}
              placeholder={t('Search in conversation')}
              disabled={!props.user}
              className='pl-9'
            />
          </div>
          <Button
            type='button'
            variant='outline'
            onClick={props.onRefresh}
            className='gap-2'
          >
            <RotateCcw className='h-4 w-4' />
            {t('Refresh')}
          </Button>
          <RealtimeStatusBadge status={props.realtimeStatus} />
        </div>
      </div>
    </CardHeader>
  )
}
