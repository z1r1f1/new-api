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
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Loader2, MessageCircle, RotateCcw, Search } from 'lucide-react'
import { nanoid } from 'nanoid'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { useAuthStore } from '@/stores/auth-store'
import { getUserAvatarStyle } from '@/lib/avatar'
import { ROLE } from '@/lib/roles'
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
  revokeChatMessage,
  sendChatMessage,
  toggleChatMessageReaction,
} from './api'
import { ChatImagePreviewDialog } from './components/chat-image-preview-dialog'
import { ChatSidebar } from './components/chat-sidebar'
import { ChatUserProfileDialog } from './components/chat-user-profile-dialog'
import { MessageComposer } from './components/message-composer'
import { MessageList } from './components/message-list'
import { RealtimeStatusBadge } from './components/realtime-status-badge'
import { useChatRealtime } from './hooks/use-chat-realtime'
import {
  filterChatUsers,
  filterConversations,
  filterMessages,
  getChatUserDisplayName,
  getChatUserInitial,
  getConversationInitial,
  getConversationTitle,
  getDirectConversationKey,
  getInitialConversation,
  getMessageSenderName,
  getTotalUnreadCount,
  MAX_MESSAGE_LENGTH,
  mergeReactionEventMessage,
  mergeMessages,
  MESSAGE_PAGE_SIZE,
  type SidebarTab,
} from './lib/format'
import {
  AVAILABLE_CHAT_STICKERS,
  buildOutgoingMessageBody,
  CHAT_IMAGE_TOTAL_BODY_MAX_LENGTH,
  CHAT_PASTED_IMAGE_MAX_COUNT,
  CHAT_STICKER_MAX_COUNT,
  createChatImageAttachment,
  getMessageReplyPreview,
  isChatMessageSendable,
  type ChatImageAttachment,
  type ChatImagePreview,
  type ChatReplyReference,
  type ChatSticker,
} from './lib/message-content'
import type {
  ApiResponse,
  ChatConversation,
  ChatEvent,
  ChatMessage,
  ChatUser,
} from './types'

interface PendingImageSend {
  conversationId: number | null
  text: string
  stickers: ChatSticker[]
  replyTo: ChatReplyReference | null
}

interface SendChatMessageVariables {
  conversationId: number
  body: string
}

interface ToggleMessageReactionVariables {
  message: ChatMessage
  emoji: string
}

export function Messaging() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const currentUserId = useAuthStore((state) => state.auth.user?.id ?? null)
  const currentUserRole = useAuthStore((state) => state.auth.user?.role ?? 0)
  const canViewUserDetails = currentUserRole >= ROLE.ADMIN
  const [sidebarTab, setSidebarTab] = useState<SidebarTab>('users')
  const [selectedUserId, setSelectedUserId] = useState<number | null>(null)
  const [activeConversationId, setActiveConversationId] = useState<
    number | null
  >(null)
  const [openingUserId, setOpeningUserId] = useState<number | null>(null)
  const [messageBody, setMessageBody] = useState('')
  const [pastedImages, setPastedImages] = useState<ChatImageAttachment[]>([])
  const [selectedStickers, setSelectedStickers] = useState<ChatSticker[]>([])
  const [replyTarget, setReplyTarget] = useState<ChatReplyReference | null>(
    null
  )
  const [processingPastedImages, setProcessingPastedImages] = useState(false)
  const [previewImage, setPreviewImage] = useState<ChatImagePreview | null>(
    null
  )
  const [profileUser, setProfileUser] = useState<ChatUser | null>(null)
  const [profileDialogOpen, setProfileDialogOpen] = useState(false)
  const [sidebarSearch, setSidebarSearch] = useState('')
  const [messageSearch, setMessageSearch] = useState('')
  const [recallingMessageId, setRecallingMessageId] = useState<number | null>(
    null
  )
  const [reactingMessageKey, setReactingMessageKey] = useState<string | null>(
    null
  )
  const [historyExhaustedByConversation, setHistoryExhaustedByConversation] =
    useState<Record<number, boolean>>({})
  const initializedConversationRef = useRef(false)
  const activeConversationIdRef = useRef<number | null>(activeConversationId)
  const pendingSendAfterImageProcessingRef = useRef<PendingImageSend | null>(
    null
  )

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
  const conversations = useMemo(
    () => conversationsQuery.data ?? [],
    [conversationsQuery.data]
  )
  const filteredUsers = useMemo(
    () => filterChatUsers(users, sidebarSearch),
    [sidebarSearch, users]
  )
  const filteredConversations = useMemo(
    () => filterConversations(conversations, sidebarSearch),
    [conversations, sidebarSearch]
  )
  const totalUnreadCount = useMemo(
    () => getTotalUnreadCount(conversations),
    [conversations]
  )
  const selectedUser = useMemo(
    () => users.find((user) => user.id === selectedUserId) ?? null,
    [selectedUserId, users]
  )
  const activeConversation = useMemo(
    () =>
      activeConversationId
        ? (conversations.find(
            (conversation) => conversation.id === activeConversationId
          ) ?? null)
        : null,
    [activeConversationId, conversations]
  )
  useEffect(() => {
    activeConversationIdRef.current = activeConversationId
  }, [activeConversationId])
  useEffect(() => {
    if (initializedConversationRef.current) return
    if (!conversationsQuery.isSuccess) return
    if (activeConversationId || selectedUserId || openingUserId) {
      initializedConversationRef.current = true
      return
    }
    const initialConversation = getInitialConversation(conversations)
    if (!initialConversation) return
    initializedConversationRef.current = true
    activeConversationIdRef.current = initialConversation.id
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setActiveConversationId(initialConversation.id)
    setSelectedUserId(initialConversation.peer?.id ?? null)
    setSidebarTab('conversations')
    setMessageBody('')
    setPastedImages([])
    setMessageSearch('')
  }, [
    activeConversationId,
    conversations,
    conversationsQuery.isSuccess,
    openingUserId,
    selectedUserId,
  ])
  const mentionUsers = useMemo(() => {
    if (!activeConversation || activeConversation.type !== 'group') return []
    return activeConversation.members.filter(
      (member) => member.id !== currentUserId
    )
  }, [activeConversation, currentUserId])

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
  const lastMessageId = messages.at(-1)?.id ?? 0
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
    (
      message: ChatMessage,
      reactionEvent?: {
        reactorUserId?: number | null
        emoji?: string
        active?: boolean
      }
    ) => {
      queryClient.setQueryData(
        messagingQueryKeys.messages(message.conversation_id),
        (current: ApiResponse<ChatMessage[]> | undefined) => {
          const currentMessages = current?.data ?? []
          const incomingMessage = reactionEvent
            ? mergeReactionEventMessage(
                currentMessages.find((item) => item.id === message.id),
                message,
                {
                  currentUserId,
                  reactorUserId: reactionEvent.reactorUserId,
                  emoji: reactionEvent.emoji,
                  active: reactionEvent.active,
                }
              )
            : message
          return {
            success: true,
            data: mergeMessages(currentMessages, [incomingMessage]),
          }
        }
      )
    },
    [currentUserId, queryClient]
  )

  const openDirectConversation = useCallback(
    (user: ChatUser, knownConversations: ChatConversation[]): boolean => {
      const directKey = getDirectConversationKey(currentUserId, user.id)
      const existingConversation = knownConversations.find(
        (conversation) =>
          conversation.type === 'direct' &&
          conversation.direct_key === directKey
      )
      if (!existingConversation) return false
      activeConversationIdRef.current = existingConversation.id
      setActiveConversationId(existingConversation.id)
      setSelectedUserId(user.id)
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
        activeConversationIdRef.current = response.data.id
        setActiveConversationId(response.data.id)
        setSelectedUserId(response.data.peer?.id ?? null)
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

  const { mutate: sendChatMessageMutation, isPending: sendingMessage } =
    useMutation({
      mutationFn: (variables: SendChatMessageVariables) => {
        return sendChatMessage(variables.conversationId, {
          body: variables.body,
          client_message_id: nanoid(),
        })
      },
      onSuccess: (response, variables) => {
        if (response.data) appendMessage(response.data)
        if (variables.conversationId === activeConversationIdRef.current) {
          setMessageBody('')
          setPastedImages([])
          setSelectedStickers([])
          setReplyTarget(null)
          setMessageSearch('')
        }
        invalidateConversations()
      },
      onError: () => {
        toast.error(t('Failed to send message'))
      },
    })

  const sendMessageWithAttachments = useCallback(
    (
      text: string,
      attachments: ChatImageAttachment[],
      stickers: ChatSticker[],
      replyTo: ChatReplyReference | null,
      conversationId: number | null
    ): boolean => {
      if (!isChatMessageSendable(text, attachments, stickers)) return false
      if (!conversationId) {
        toast.error(t('Select a conversation first'))
        return false
      }
      const textBody = text.trim()
      if (textBody.length > MAX_MESSAGE_LENGTH) {
        toast.error(t('Message is too long'))
        return false
      }
      const body = buildOutgoingMessageBody(
        text,
        attachments,
        replyTo,
        stickers
      )
      if (body.length > CHAT_IMAGE_TOTAL_BODY_MAX_LENGTH) {
        toast.error(t('Message is too long'))
        return false
      }
      sendChatMessageMutation({ conversationId, body })
      return true
    },
    [sendChatMessageMutation, t]
  )

  const revokeMutation = useMutation({
    mutationFn: (message: ChatMessage) =>
      revokeChatMessage(message.conversation_id, message.id),
    onMutate: (message) => {
      setRecallingMessageId(message.id)
    },
    onSuccess: (response) => {
      if (response.data) appendMessage(response.data)
      toast.success(t('Message recalled'))
      invalidateConversations()
    },
    onError: () => {
      toast.error(t('Failed to recall message'))
    },
    onSettled: () => {
      setRecallingMessageId(null)
    },
  })

  const reactionMutation = useMutation({
    mutationFn: (variables: ToggleMessageReactionVariables) =>
      toggleChatMessageReaction(
        variables.message.conversation_id,
        variables.message.id,
        { emoji: variables.emoji }
      ),
    onMutate: (variables) => {
      setReactingMessageKey(`${variables.message.id}:${variables.emoji}`)
    },
    onSuccess: (response) => {
      if (response.data) appendMessage(response.data)
    },
    onError: () => {
      toast.error(t('Failed to update reaction'))
    },
    onSettled: () => {
      setReactingMessageKey(null)
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
      if (event.type === 'message.revoked' && event.message) {
        appendMessage(event.message)
        invalidateConversations()
        return
      }
      if (event.type === 'message.reaction.updated' && event.message) {
        appendMessage(event.message, {
          reactorUserId: event.user_id,
          emoji: event.reaction_emoji,
          active: event.reaction_active,
        })
        return
      }
      if (event.type === 'message.read') {
        invalidateConversations()
        if (event.conversation_id) {
          void queryClient.invalidateQueries({
            queryKey: messagingQueryKeys.messages(event.conversation_id),
          })
        }
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
    [appendMessage, invalidateConversations, queryClient]
  )

  const realtimeStatus = useChatRealtime({
    userId: currentUserId,
    conversationId: activeConversationId,
    onEvent: handleRealtimeEvent,
  })

  useEffect(() => {
    if (!activeConversationId || !lastMessageId) return
    void markChatRead(activeConversationId, {
      last_read_message_id: lastMessageId,
    }).then(() => invalidateConversations())
  }, [activeConversationId, invalidateConversations, lastMessageId])

  useEffect(() => {
    if (processingPastedImages) return
    const pendingSend = pendingSendAfterImageProcessingRef.current
    if (!pendingSend) return
    pendingSendAfterImageProcessingRef.current = null
    if (pendingSend.conversationId !== activeConversationIdRef.current) return
    sendMessageWithAttachments(
      pendingSend.text,
      pastedImages,
      pendingSend.stickers,
      pendingSend.replyTo,
      pendingSend.conversationId
    )
  }, [pastedImages, processingPastedImages, sendMessageWithAttachments])

  const handleSelectUser = (user: ChatUser): void => {
    pendingSendAfterImageProcessingRef.current = null
    activeConversationIdRef.current = null
    setSelectedUserId(user.id)
    setMessageBody('')
    setPastedImages([])
    setSelectedStickers([])
    setReplyTarget(null)
    setPreviewImage(null)
    setMessageSearch('')
    setActiveConversationId(null)
    if (openDirectConversation(user, conversations)) return
    createDirectMutation.mutate(user.id)
  }

  const handleSelectConversation = (conversation: ChatConversation): void => {
    pendingSendAfterImageProcessingRef.current = null
    activeConversationIdRef.current = conversation.id
    setActiveConversationId(conversation.id)
    setSelectedUserId(conversation.peer?.id ?? null)
    setMessageBody('')
    setPastedImages([])
    setSelectedStickers([])
    setReplyTarget(null)
    setPreviewImage(null)
    setMessageSearch('')
  }

  const handleViewUser = (user: ChatUser): void => {
    setProfileUser(user)
    setProfileDialogOpen(true)
  }

  const handleStartDirectChat = (user: ChatUser): void => {
    setProfileDialogOpen(false)
    handleSelectUser(user)
  }

  const handleReplyMessage = (message: ChatMessage): void => {
    setReplyTarget({
      messageId: message.id,
      senderName: getMessageSenderName(message),
      preview: getMessageReplyPreview(message.body) || t('Message'),
    })
  }

  const handlePasteImages = (files: File[]): void => {
    const pasteConversationId = activeConversationIdRef.current
    if (pastedImages.length >= CHAT_PASTED_IMAGE_MAX_COUNT) {
      toast.error(
        t('You can attach up to {{count}} images', {
          count: CHAT_PASTED_IMAGE_MAX_COUNT,
        })
      )
      return
    }
    const availableSlots = CHAT_PASTED_IMAGE_MAX_COUNT - pastedImages.length
    const filesToProcess = files.slice(0, availableSlots)
    if (files.length > availableSlots) {
      toast.error(
        t('You can attach up to {{count}} images', {
          count: CHAT_PASTED_IMAGE_MAX_COUNT,
        })
      )
    }

    setProcessingPastedImages(true)
    void Promise.all(
      filesToProcess.map((file) => createChatImageAttachment(file, nanoid()))
    )
      .then((attachments) => {
        if (pasteConversationId !== activeConversationIdRef.current) {
          pendingSendAfterImageProcessingRef.current = null
          return
        }
        setPastedImages((current) => [...current, ...attachments])
      })
      .catch((error: unknown) => {
        pendingSendAfterImageProcessingRef.current = null
        const message = error instanceof Error ? error.message : ''
        toast.error(t(message || 'Failed to read pasted image'))
      })
      .finally(() => setProcessingPastedImages(false))
  }

  const handleRemovePastedImage = (id: string): void => {
    setPastedImages((current) => current.filter((image) => image.id !== id))
  }

  const handleSelectSticker = (sticker: ChatSticker): void => {
    setSelectedStickers((current) => {
      if (current.length >= CHAT_STICKER_MAX_COUNT) {
        toast.error(
          t('You can select up to {{count}} stickers', {
            count: CHAT_STICKER_MAX_COUNT,
          })
        )
        return current
      }
      return [...current, sticker]
    })
  }

  const handleRemoveSticker = (index: number): void => {
    setSelectedStickers((current) =>
      current.filter((_, currentIndex) => currentIndex !== index)
    )
  }

  const handleReactMessage = (message: ChatMessage, emoji: string): void => {
    reactionMutation.mutate({ message, emoji })
  }

  const handleSendMessage = (): void => {
    if (processingPastedImages) {
      pendingSendAfterImageProcessingRef.current = {
        conversationId: activeConversationId,
        text: messageBody,
        stickers: selectedStickers,
        replyTo: replyTarget,
      }
      return
    }
    sendMessageWithAttachments(
      messageBody,
      pastedImages,
      selectedStickers,
      replyTarget,
      activeConversationId
    )
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
      <div className='grid h-full min-h-0 flex-1 gap-4 lg:grid-cols-[20rem_minmax(0,1fr)]'>
        <aside className='h-full min-h-0'>
          <ChatSidebar
            activeTab={sidebarTab}
            users={filteredUsers}
            conversations={filteredConversations}
            activeUserId={selectedUserId}
            activeConversationId={activeConversationId}
            loadingUsers={usersQuery.isPending}
            loadingConversations={conversationsQuery.isPending}
            searchText={sidebarSearch}
            openingUserId={openingUserId}
            totalUnreadCount={totalUnreadCount}
            onTabChange={setSidebarTab}
            onSearchChange={setSidebarSearch}
            onSelectUser={handleSelectUser}
            onSelectConversation={handleSelectConversation}
            onRefresh={handleRefresh}
          />
        </aside>

        <main className='h-full min-h-0 min-w-0'>
          <Card className='border-border/80 flex h-full min-h-0 overflow-hidden shadow-sm'>
            <div className='flex min-h-0 min-w-0 flex-1 flex-col'>
              <ChatHeader
                conversation={activeConversation}
                pendingUser={selectedUser}
                realtimeStatus={realtimeStatus}
                messageCount={messages.length}
                searchText={messageSearch}
                loading={loadingThread}
                onSearchChange={setMessageSearch}
                onRefresh={handleRefresh}
                onViewUser={handleViewUser}
              />
              <CardContent className='flex min-h-0 flex-1 flex-col p-0'>
                <MessageList
                  conversation={activeConversation}
                  messages={filteredMessages}
                  allMessageCount={messages.length}
                  currentUserId={currentUserId}
                  loading={messagesQuery.isPending || loadingThread}
                  empty={!activeConversationId}
                  searchText={messageSearch}
                  canLoadOlder={canLoadOlder}
                  loadingOlder={loadOlderMutation.isPending}
                  recallingMessageId={recallingMessageId}
                  reactingMessageKey={reactingMessageKey}
                  onLoadOlder={() => loadOlderMutation.mutate()}
                  onRecallMessage={(message) => revokeMutation.mutate(message)}
                  onReplyMessage={handleReplyMessage}
                  onReactMessage={handleReactMessage}
                  onViewUser={handleViewUser}
                />
                <MessageComposer
                  value={messageBody}
                  active={Boolean(activeConversationId)}
                  sending={sendingMessage}
                  processingImages={processingPastedImages}
                  maxLength={MAX_MESSAGE_LENGTH}
                  mentionUsers={mentionUsers}
                  attachments={pastedImages}
                  stickers={selectedStickers}
                  availableStickers={AVAILABLE_CHAT_STICKERS}
                  replyTo={replyTarget}
                  onChange={setMessageBody}
                  onSend={handleSendMessage}
                  onPasteImages={handlePasteImages}
                  onRemoveAttachment={handleRemovePastedImage}
                  onPreviewAttachment={(attachment) =>
                    setPreviewImage({
                      src: attachment.dataUrl,
                      alt: attachment.name,
                    })
                  }
                  onSelectSticker={handleSelectSticker}
                  onRemoveSticker={handleRemoveSticker}
                  onCancelReply={() => setReplyTarget(null)}
                />
              </CardContent>
            </div>
          </Card>
        </main>
      </div>
      <ChatUserProfileDialog
        user={profileUser}
        open={profileDialogOpen}
        canViewDetails={canViewUserDetails}
        currentUserId={currentUserId}
        openingDirect={Boolean(profileUser && openingUserId === profileUser.id)}
        onOpenChange={setProfileDialogOpen}
        onStartDirectChat={handleStartDirectChat}
      />
      <ChatImagePreviewDialog
        image={previewImage}
        open={Boolean(previewImage)}
        onOpenChange={(open) => {
          if (!open) setPreviewImage(null)
        }}
      />
    </div>
  )
}

interface ChatHeaderProps {
  conversation: ChatConversation | null
  pendingUser: ChatUser | null
  realtimeStatus: string
  messageCount: number
  searchText: string
  loading: boolean
  onSearchChange: (value: string) => void
  onRefresh: () => void
  onViewUser: (user: ChatUser) => void
}

function ChatHeader(props: ChatHeaderProps) {
  const { t } = useTranslation()
  const active = Boolean(props.conversation)
  const title = getHeaderTitle(props.conversation, props.pendingUser, t)
  const subtitle = getHeaderSubtitle(props.conversation, props.pendingUser, t)
  const headerUser = props.conversation?.peer ?? props.pendingUser
  const headerUserName = headerUser ? getChatUserDisplayName(headerUser) : ''

  return (
    <CardHeader className='border-border/80 shrink-0 border-b'>
      <div className='flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between'>
        <div className='flex min-w-0 items-center gap-3'>
          {headerUser ? (
            <button
              type='button'
              onClick={() => props.onViewUser(headerUser)}
              className='focus-visible:ring-ring rounded-full transition-transform outline-none hover:scale-105 focus-visible:ring-2 focus-visible:ring-offset-2'
              aria-label={t('View user profile')}
            >
              <Avatar size='lg' className='shadow-sm'>
                <AvatarFallback
                  className='font-semibold'
                  style={getUserAvatarStyle(headerUserName)}
                >
                  {getChatUserInitial(headerUser)}
                </AvatarFallback>
              </Avatar>
            </button>
          ) : props.conversation ? (
            <Avatar size='lg' className='shadow-sm'>
              <AvatarFallback
                className='font-semibold'
                style={getUserAvatarStyle(
                  getConversationTitle(props.conversation)
                )}
              >
                {getConversationInitial(props.conversation)}
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
              <span className='truncate'>{subtitle}</span>
              {active && (
                <>
                  <span>·</span>
                  <span>
                    {t('{{count}} messages', { count: props.messageCount })}
                  </span>
                </>
              )}
              {props.loading && (
                <>
                  <span>·</span>
                  <Loader2 className='h-3 w-3 animate-spin' />
                </>
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
              disabled={!active}
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

function getHeaderTitle(
  conversation: ChatConversation | null,
  pendingUser: ChatUser | null,
  translate: (key: string, options?: Record<string, unknown>) => string
): string {
  if (conversation) {
    const title = getConversationTitle(conversation)
    return conversation.is_default ? translate(title) : title
  }
  if (pendingUser) {
    return translate('Chat with {{name}}', { name: `@${pendingUser.username}` })
  }
  return translate('Select a conversation')
}

function getHeaderSubtitle(
  conversation: ChatConversation | null,
  pendingUser: ChatUser | null,
  translate: (key: string) => string
): string {
  if (conversation?.is_default) return translate('Everyone is in this group')
  if (conversation?.type === 'group') return translate('Group conversation')
  if (conversation?.peer) return getChatUserDisplayName(conversation.peer)
  if (pendingUser) return getChatUserDisplayName(pendingUser)
  return translate(
    'Choose a user or conversation from the left to start chatting.'
  )
}
