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
import { Sparkles } from 'lucide-react'
import { nanoid } from 'nanoid'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { useAuthStore } from '@/stores/auth-store'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'
import {
  addChatMember,
  createDirectConversation,
  createGroupConversation,
  listChatConversations,
  listChatMessages,
  markChatRead,
  messagingQueryKeys,
  removeChatMember,
  sendChatMessage,
} from './api'
import { ConversationDetailsPanel } from './components/conversation-details-panel'
import { ConversationHeader } from './components/conversation-header'
import { ConversationSidebar } from './components/conversation-sidebar'
import { MessageComposer } from './components/message-composer'
import { MessageList } from './components/message-list'
import { RealtimeStatusBadge } from './components/realtime-status-badge'
import { StartConversationPanel } from './components/start-conversation-panel'
import { useChatRealtime } from './hooks/use-chat-realtime'
import {
  type ConversationFilter,
  filterMessages,
  getConversationFallbackTitle,
  MAX_MESSAGE_LENGTH,
  mergeMessages,
  MESSAGE_PAGE_SIZE,
  parseMemberIds,
} from './lib/format'
import type { ApiResponse, ChatEvent, ChatMessage } from './types'

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
  const [removeMemberUserId, setRemoveMemberUserId] = useState('')
  const [conversationSearch, setConversationSearch] = useState('')
  const [conversationFilter, setConversationFilter] =
    useState<ConversationFilter>('all')
  const [messageSearch, setMessageSearch] = useState('')
  const [historyExhaustedByConversation, setHistoryExhaustedByConversation] =
    useState<Record<number, boolean>>({})

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
  const filteredConversations = useMemo(() => {
    const keyword = conversationSearch.trim().toLowerCase()
    return conversations.filter((conversation) => {
      if (
        conversationFilter !== 'all' &&
        conversation.type !== conversationFilter
      ) {
        return false
      }
      if (!keyword) return true
      const title = getConversationFallbackTitle(conversation).toLowerCase()
      return (
        title.includes(keyword) || String(conversation.id).includes(keyword)
      )
    })
  }, [conversationFilter, conversationSearch, conversations])

  const conversationStats = useMemo(() => {
    const directCount = conversations.filter(
      (conversation) => conversation.type === 'direct'
    ).length
    const groupCount = conversations.filter(
      (conversation) => conversation.type === 'group'
    ).length
    const activeCount = conversations.filter(
      (conversation) => conversation.last_message_id > 0
    ).length
    return { directCount, groupCount, activeCount, total: conversations.length }
  }, [conversations])

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

  const handleRealtimeEvent = useCallback(
    (event: ChatEvent) => {
      if (event.type === 'message.created' && event.message) {
        appendMessage(event.message)
        invalidateConversations()
        return
      }
      if (
        event.type === 'conversation.created' ||
        event.type === 'conversation.updated' ||
        event.type === 'member.added' ||
        event.type === 'member.removed'
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
      setMessageSearch('')
      invalidateConversations()
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
      invalidateConversations()
      toast.success(t('Member added'))
    },
  })

  const removeMemberMutation = useMutation({
    mutationFn: () => {
      if (!activeConversationId)
        throw new Error(t('Select a conversation first'))
      return removeChatMember(
        activeConversationId,
        Number(removeMemberUserId.trim())
      )
    },
    onSuccess: () => {
      setRemoveMemberUserId('')
      invalidateConversations()
      toast.success(t('Member removed'))
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
    const body = messageBody.trim()
    if (!body) return
    if (body.length > MAX_MESSAGE_LENGTH) {
      toast.error(t('Message is too long'))
      return
    }
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

  const handleRemoveMember = (): void => {
    const userId = Number(removeMemberUserId.trim())
    if (!Number.isInteger(userId) || userId <= 0) {
      toast.error(t('Enter a valid user ID'))
      return
    }
    removeMemberMutation.mutate()
  }

  const handleRefresh = (): void => {
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

  return (
    <div className='from-background via-muted/20 to-background flex h-full min-h-0 flex-col bg-[radial-gradient(circle_at_top_left,var(--tw-gradient-stops))] p-3 lg:p-4'>
      <MessagingHero
        realtimeStatus={realtimeStatus}
        total={conversationStats.total}
        groupCount={conversationStats.groupCount}
        activeCount={conversationStats.activeCount}
      />
      <div className='grid min-h-0 flex-1 gap-4 xl:grid-cols-[22rem_minmax(0,1fr)_20rem]'>
        <aside className='flex min-h-0 flex-col gap-4'>
          <ConversationSidebar
            conversations={filteredConversations}
            allConversationCount={conversations.length}
            activeConversationId={activeConversationId}
            loading={conversationsQuery.isPending}
            searchText={conversationSearch}
            filter={conversationFilter}
            directCount={conversationStats.directCount}
            groupCount={conversationStats.groupCount}
            onSearchChange={setConversationSearch}
            onFilterChange={setConversationFilter}
            onSelect={setActiveConversationId}
            onRefresh={handleRefresh}
          />
          <StartConversationPanel
            directUserId={directUserId}
            groupTitle={groupTitle}
            groupMemberIds={groupMemberIds}
            creatingDirect={createDirectMutation.isPending}
            creatingGroup={createGroupMutation.isPending}
            onDirectUserIdChange={setDirectUserId}
            onGroupTitleChange={setGroupTitle}
            onGroupMemberIdsChange={setGroupMemberIds}
            onCreateDirect={handleCreateDirect}
            onCreateGroup={handleCreateGroup}
          />
        </aside>

        <main className='min-w-0'>
          <Card className='border-border/80 bg-background/95 flex h-full min-h-0 overflow-hidden shadow-sm'>
            <div className='flex min-w-0 flex-1 flex-col'>
              <ConversationHeader
                conversation={activeConversation}
                realtimeStatus={realtimeStatus}
                messageCount={messages.length}
                searchText={messageSearch}
                onSearchChange={setMessageSearch}
                onRefresh={handleRefresh}
              />
              <CardContent className='flex min-h-0 flex-1 flex-col p-0'>
                <MessageList
                  messages={filteredMessages}
                  allMessageCount={messages.length}
                  currentUserId={currentUserId}
                  loading={messagesQuery.isPending}
                  empty={!activeConversation}
                  searchText={messageSearch}
                  canLoadOlder={canLoadOlder}
                  loadingOlder={loadOlderMutation.isPending}
                  onLoadOlder={() => loadOlderMutation.mutate()}
                />
                <MessageComposer
                  value={messageBody}
                  active={Boolean(activeConversation)}
                  sending={sendMutation.isPending}
                  maxLength={MAX_MESSAGE_LENGTH}
                  onChange={setMessageBody}
                  onSend={handleSendMessage}
                />
              </CardContent>
            </div>
          </Card>
        </main>

        <aside className='hidden min-h-0 xl:flex'>
          <ConversationDetailsPanel
            conversation={activeConversation}
            currentUserId={currentUserId}
            realtimeStatus={realtimeStatus}
            messageCount={messages.length}
            memberUserId={memberUserId}
            removeMemberUserId={removeMemberUserId}
            addingMember={addMemberMutation.isPending}
            removingMember={removeMemberMutation.isPending}
            onMemberUserIdChange={setMemberUserId}
            onRemoveMemberUserIdChange={setRemoveMemberUserId}
            onAddMember={handleAddMember}
            onRemoveMember={handleRemoveMember}
          />
        </aside>
      </div>
    </div>
  )
}

interface MessagingHeroProps {
  realtimeStatus: string
  total: number
  groupCount: number
  activeCount: number
}

function MessagingHero(props: MessagingHeroProps) {
  const { t } = useTranslation()

  return (
    <div className='bg-background/80 mb-4 flex flex-col gap-3 rounded-3xl border p-4 shadow-sm backdrop-blur md:flex-row md:items-center md:justify-between'>
      <div className='min-w-0'>
        <div className='mb-2 flex flex-wrap items-center gap-2'>
          <Badge variant='secondary' className='gap-1 rounded-full'>
            <Sparkles className='h-3.5 w-3.5' />
            {t('Enterprise messaging')}
          </Badge>
          <RealtimeStatusBadge status={props.realtimeStatus} />
        </div>
        <h1 className='text-foreground text-2xl font-semibold tracking-tight'>
          {t('Messages command center')}
        </h1>
        <p className='text-muted-foreground mt-1 text-sm'>
          {t(
            'Coordinate direct and group conversations with realtime delivery, message history, and member operations.'
          )}
        </p>
      </div>
      <div className='grid grid-cols-3 gap-2 md:w-[26rem]'>
        <MetricTile label={t('All chats')} value={props.total} />
        <MetricTile label={t('Groups')} value={props.groupCount} />
        <MetricTile label={t('Active chats')} value={props.activeCount} />
      </div>
    </div>
  )
}

interface MetricTileProps {
  label: string
  value: number
}

function MetricTile(props: MetricTileProps) {
  return (
    <div className='bg-muted/30 rounded-2xl border px-3 py-2'>
      <div className='text-muted-foreground text-[11px] font-medium tracking-wide uppercase'>
        {props.label}
      </div>
      <div className='text-xl font-semibold tabular-nums'>{props.value}</div>
    </div>
  )
}
