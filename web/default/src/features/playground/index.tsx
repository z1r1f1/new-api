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
import { useQuery } from '@tanstack/react-query'
import {
  Bug,
  MessageSquare,
  PanelLeftClose,
  Pencil,
  Plus,
  Settings2,
  Trash2,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { useAuthStore } from '@/stores/auth-store'
import { cn } from '@/lib/utils'
import { useIsMobile } from '@/hooks/use-mobile'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { getUserModelsByGroup, getUserGroups } from './api'
import { PlaygroundChat } from './components/playground-chat'
import { PlaygroundDebugPanel } from './components/playground-debug-panel'
import { PlaygroundInput } from './components/playground-input'
import { PlaygroundSettingsPanel } from './components/playground-settings-panel'
import {
  DEFAULT_CONFIG,
  DEFAULT_GROUP,
  DEFAULT_PARAMETER_ENABLED,
} from './constants'
import { usePlaygroundState, useChatHandler } from './hooks'
import {
  buildPlaygroundPreviewPayload,
  clearPlaygroundData,
  createLoadingAssistantMessage,
  markActivePlaygroundChatMessage,
  createPlaygroundSession,
  createUserMessage,
  exportPlaygroundData,
  importPlaygroundData,
  parseCustomRequestBody,
  type PlaygroundStorageScope,
} from './lib'
import type { Message as MessageType, PlaygroundWorkbenchState } from './types'

const defaultWorkbenchState: PlaygroundWorkbenchState = {
  showSettings: true,
  showDebugPanel: false,
  customRequestMode: false,
  customRequestBody: '',
  searchEnabled: false,
}

export function Playground() {
  const storageUserId = useAuthStore((state) => state.auth.user?.id ?? null)
  const storageKey =
    storageUserId === null ? 'anonymous' : String(storageUserId)

  return <PlaygroundContent key={storageKey} storageUserId={storageUserId} />
}

interface PlaygroundContentProps {
  storageUserId: PlaygroundStorageScope
}

function PlaygroundContent(props: PlaygroundContentProps) {
  const { t } = useTranslation()
  const isMobile = useIsMobile()
  const storageUserId = props.storageUserId
  const [mobileSettingsOpen, setMobileSettingsOpen] = useState(false)
  const [mobileDebugOpen, setMobileDebugOpen] = useState(false)
  const [sessionSidebarOpen, setSessionSidebarOpen] = useState(true)
  const [editingSessionId, setEditingSessionId] = useState<string | null>(null)
  const {
    config,
    parameterEnabled,
    workbenchState,
    messages,
    sessions,
    activeSessionId,
    models,
    groups,
    debugData,
    activeDebugTab,
    updateMessages,
    updateSessionMessages,
    setModels,
    setGroups,
    setDebugData,
    setActiveDebugTab,
    updateConfig,
    updateParameterEnabled,
    updateWorkbenchState,
    clearMessages,
    replaceWorkbenchState,
    resetConfig,
    replaceConfig,
    replaceParameterEnabled,
    resetDebugData,
    createSession,
    renameSession,
    deleteSession,
    replaceSessions,
    switchSession,
  } = usePlaygroundState(storageUserId)

  const { sendChat, stopGeneration, isGenerating } = useChatHandler({
    config,
    parameterEnabled,
    messages,
    activeSessionId,
    storageUserId,
    onMessageUpdate: updateMessages,
    onSessionMessageUpdate: updateSessionMessages,
    onDebugUpdate: setDebugData,
    onDebugTabChange: setActiveDebugTab,
    searchEnabled: workbenchState.searchEnabled,
  })

  // Edit dialog state
  const [editingMessageKey, setEditingMessageKey] = useState<string | null>(
    null
  )
  const activeSession = sessions.find(
    (session) => session.id === activeSessionId
  )

  // Load models
  const { data: modelsData, isLoading: isLoadingModels } = useQuery({
    queryKey: ['playground-models', config.group],
    queryFn: () => getUserModelsByGroup(config.group),
  })

  // Load groups
  const { data: groupsData, isLoading: isLoadingGroups } = useQuery({
    queryKey: ['playground-groups'],
    queryFn: getUserGroups,
  })

  // Update models when data changes
  useEffect(() => {
    if (!modelsData) return

    setModels(modelsData)

    // Set default model if current model is not available
    const isCurrentModelValid = modelsData.some((m) => m.value === config.model)
    if (modelsData.length > 0 && !isCurrentModelValid) {
      updateConfig('model', modelsData[0].value)
    }
  }, [modelsData, config.model, setModels, updateConfig])

  // Update groups when data changes
  useEffect(() => {
    if (!groupsData) return

    setGroups(groupsData)

    const hasCurrentGroup = groupsData.some((g) => g.value === config.group)
    if (!hasCurrentGroup && groupsData.length > 0) {
      const fallback =
        groupsData.find((g) => g.value === DEFAULT_GROUP)?.value ??
        groupsData[0].value
      updateConfig('group', fallback)
    }
  }, [groupsData, setGroups, config.group, updateConfig])

  const previewResult = useMemo(
    () =>
      buildPlaygroundPreviewPayload({
        messages,
        config,
        parameterEnabled,
        customRequestMode: workbenchState.customRequestMode,
        customRequestBody: workbenchState.customRequestBody,
        searchEnabled: workbenchState.searchEnabled,
      }),
    [
      messages,
      config,
      parameterEnabled,
      workbenchState.customRequestMode,
      workbenchState.customRequestBody,
      workbenchState.searchEnabled,
    ]
  )

  useEffect(() => {
    const previewRequest = previewResult.error
      ? { error: previewResult.error }
      : previewResult.payload
    setDebugData((prev) => ({
      ...prev,
      previewRequest,
      previewTimestamp: new Date().toISOString(),
    }))
  }, [previewResult, setDebugData])

  const handleSendMessage = (text: string) => {
    if (workbenchState.customRequestMode) {
      const customResult = parseCustomRequestBody(
        workbenchState.customRequestBody
      )
      if (customResult.error || !customResult.payload) {
        toast.error(
          customResult.error
            ? t('JSON error: {{error}}', { error: customResult.error })
            : t('Custom request body is empty')
        )
        return
      }

      const userMessage = createUserMessage(text)
      const assistantMessage = createLoadingAssistantMessage()
      const newMessages = [...messages, userMessage, assistantMessage]
      markActivePlaygroundChatMessage(assistantMessage.key, storageUserId)
      updateMessages(newMessages)
      sendChat(newMessages, customResult.payload)
      return
    }

    const userMessage = createUserMessage(text)
    const assistantMessage = createLoadingAssistantMessage()

    const newMessages = [...messages, userMessage, assistantMessage]
    markActivePlaygroundChatMessage(assistantMessage.key, storageUserId)
    updateMessages(newMessages)

    // Send chat request
    sendChat(newMessages)
  }

  const handleRegenerateMessage = (message: MessageType) => {
    // Find the message index and regenerate from there
    const messageIndex = messages.findIndex((m) => m.key === message.key)
    if (messageIndex === -1) return

    // Remove messages after this one and regenerate
    const messagesUpToHere = messages.slice(0, messageIndex)
    const loadingMessage = createLoadingAssistantMessage()
    const newMessages = [...messagesUpToHere, loadingMessage]

    markActivePlaygroundChatMessage(loadingMessage.key, storageUserId)
    updateMessages(newMessages)
    sendChat(newMessages)
  }

  const handleEditMessage = useCallback((message: MessageType) => {
    setEditingMessageKey(message.key)
  }, [])

  const handleEditOpenChange = useCallback((open: boolean) => {
    if (!open) setEditingMessageKey(null)
  }, [])

  // Apply edit and optionally re-submit from the edited user message
  const applyEdit = useCallback(
    (newContent: string, submit: boolean) => {
      if (!editingMessageKey) return
      const index = messages.findIndex((m) => m.key === editingMessageKey)
      if (index === -1) return

      const updated = messages.map((m) =>
        m.key === editingMessageKey
          ? { ...m, versions: [{ ...m.versions[0], content: newContent }] }
          : m
      )

      setEditingMessageKey(null)

      if (!submit || updated[index].from !== 'user') {
        updateMessages(updated)
        return
      }

      const loadingMsg = createLoadingAssistantMessage()
      const toSubmit = [...updated.slice(0, index + 1), loadingMsg]
      markActivePlaygroundChatMessage(loadingMsg.key, storageUserId)
      updateMessages(toSubmit)
      sendChat(toSubmit)
    },
    [editingMessageKey, messages, updateMessages, sendChat, storageUserId]
  )

  const handleDeleteMessage = (message: MessageType) => {
    const newMessages = messages.filter((m) => m.key !== message.key)
    updateMessages(newMessages)
  }

  const handleClearChatHistory = useCallback(() => {
    clearMessages()
    resetDebugData()
  }, [clearMessages, resetDebugData])

  const handleExport = useCallback(() => {
    exportPlaygroundData({
      config,
      parameterEnabled,
      workbenchState,
      messages,
      sessions,
      activeSessionId,
    })
    toast.success(t('Playground configuration exported'))
  }, [
    config,
    parameterEnabled,
    workbenchState,
    messages,
    sessions,
    activeSessionId,
    t,
  ])

  const handleImport = useCallback(
    async (file: File) => {
      try {
        const imported = await importPlaygroundData(file)
        const importedConfig = imported.config || imported.inputs
        if (importedConfig) replaceConfig(importedConfig)
        if (imported.parameterEnabled) {
          replaceParameterEnabled(imported.parameterEnabled)
        }
        const importedWorkbenchState: Partial<PlaygroundWorkbenchState> = {}
        if (typeof imported.customRequestMode === 'boolean') {
          importedWorkbenchState.customRequestMode = imported.customRequestMode
        }
        if (typeof imported.customRequestBody === 'string') {
          importedWorkbenchState.customRequestBody = imported.customRequestBody
        }
        if (typeof imported.showDebugPanel === 'boolean') {
          importedWorkbenchState.showDebugPanel = imported.showDebugPanel
        }
        if (typeof imported.searchEnabled === 'boolean') {
          importedWorkbenchState.searchEnabled = imported.searchEnabled
        }
        replaceWorkbenchState(importedWorkbenchState)
        if (imported.sessions?.length) {
          replaceSessions(imported.sessions, imported.activeSessionId)
        } else if (imported.messages) {
          updateMessages(imported.messages)
        }
        toast.success(t('Playground configuration imported'))
      } catch (error) {
        toast.error(
          error instanceof Error
            ? error.message
            : t('Failed to import Playground configuration')
        )
      }
    },
    [
      replaceConfig,
      replaceParameterEnabled,
      replaceWorkbenchState,
      replaceSessions,
      updateMessages,
      t,
    ]
  )

  const handleReset = useCallback(() => {
    if (!window.confirm(t('Reset Playground settings and messages?'))) return
    clearPlaygroundData(storageUserId)
    resetConfig()
    replaceConfig(DEFAULT_CONFIG)
    replaceParameterEnabled(DEFAULT_PARAMETER_ENABLED)
    replaceWorkbenchState(defaultWorkbenchState)
    replaceSessions([createPlaygroundSession()])
    resetDebugData()
    toast.success(t('Playground reset complete'))
  }, [
    resetConfig,
    replaceConfig,
    replaceParameterEnabled,
    replaceWorkbenchState,
    replaceSessions,
    resetDebugData,
    storageUserId,
    t,
  ])

  const sessionSidebar = (
    <aside
      className={cn(
        'bg-background flex shrink-0 flex-col overflow-hidden border-r transition-all duration-200',
        sessionSidebarOpen ? 'w-56' : 'w-0 border-r-0'
      )}
    >
      <div className='border-border flex h-12 shrink-0 items-center justify-between border-b px-3'>
        <span
          className={cn(
            'truncate text-sm font-medium',
            !sessionSidebarOpen && 'hidden'
          )}
        >
          {t('Sessions')}
        </span>
        <Button
          size='icon'
          variant='ghost'
          className='size-7 shrink-0'
          onClick={() => setSessionSidebarOpen(false)}
          aria-label={t('Close session sidebar')}
        >
          <PanelLeftClose className='size-4' />
        </Button>
      </div>
      <div
        className={cn(
          'flex-1 overflow-y-auto',
          !sessionSidebarOpen && 'hidden'
        )}
      >
        {sessions.map((session) => {
          const isActive = session.id === activeSessionId
          const isEditing = editingSessionId === session.id
          return (
            <div
              key={session.id}
              className={cn(
                'group border-border/50 flex items-center gap-1 border-b px-2 py-1.5 transition-colors',
                isActive && 'bg-accent'
              )}
            >
              {isEditing ? (
                <Input
                  autoFocus
                  className='h-7 flex-1 text-xs'
                  defaultValue={session.title || t('New session')}
                  onBlur={(e) => {
                    const newTitle = e.currentTarget.value.trim()
                    if (newTitle && newTitle !== session.title) {
                      renameSession(session.id, newTitle)
                    }
                    setEditingSessionId(null)
                  }}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') {
                      const newTitle = e.currentTarget.value.trim()
                      if (newTitle && newTitle !== session.title) {
                        renameSession(session.id, newTitle)
                      }
                      setEditingSessionId(null)
                    }
                    if (e.key === 'Escape') {
                      setEditingSessionId(null)
                    }
                  }}
                />
              ) : (
                <button
                  type='button'
                  className={cn(
                    'flex min-w-0 flex-1 items-center gap-2 truncate py-0.5 text-left text-sm',
                    isActive && 'font-medium'
                  )}
                  onClick={() => switchSession(session.id)}
                >
                  <MessageSquare className='text-muted-foreground size-3.5 shrink-0' />
                  <span className='truncate'>
                    {session.title || t('New session')}
                  </span>
                </button>
              )}
              {!isEditing && (
                <div className='flex shrink-0 opacity-0 transition-opacity group-hover:opacity-100'>
                  <Button
                    size='icon'
                    variant='ghost'
                    className='size-6'
                    onClick={(e) => {
                      e.stopPropagation()
                      setEditingSessionId(session.id)
                    }}
                    aria-label={t('Rename session')}
                  >
                    <Pencil className='size-3' />
                  </Button>
                  <Button
                    size='icon'
                    variant='ghost'
                    className='text-destructive hover:text-destructive size-6'
                    onClick={(e) => {
                      e.stopPropagation()
                      deleteSession(session.id)
                    }}
                    aria-label={t('Delete session')}
                  >
                    <Trash2 className='size-3' />
                  </Button>
                </div>
              )}
            </div>
          )
        })}
      </div>
      <div className={cn('border-t p-2', !sessionSidebarOpen && 'hidden')}>
        <Button
          type='button'
          variant='outline'
          size='sm'
          className='w-full'
          onClick={createSession}
        >
          <Plus className='mr-2 size-4' />
          {t('New session')}
        </Button>
      </div>
    </aside>
  )

  const sessionHeader = (
    <div className='border-border bg-background/95 flex shrink-0 items-center border-b px-3 py-2'>
      {!sessionSidebarOpen && (
        <Button
          size='icon'
          variant='ghost'
          className='size-7 shrink-0'
          onClick={() => setSessionSidebarOpen(true)}
          aria-label={t('Open session sidebar')}
        >
          <PanelLeftClose className='size-4 rotate-180' />
        </Button>
      )}
      <span className='ml-2 truncate text-sm font-medium'>
        {activeSession?.title || t('New session')}
      </span>
    </div>
  )

  // session controls moved to sidebar

  const settingsPanel = (
    <PlaygroundSettingsPanel
      config={config}
      parameterEnabled={parameterEnabled}
      workbenchState={workbenchState}
      models={models}
      groups={groups}
      messages={messages}
      disabled={isGenerating || isLoadingModels || isLoadingGroups}
      onConfigChange={updateConfig}
      onParameterEnabledChange={updateParameterEnabled}
      onWorkbenchChange={updateWorkbenchState}
      onExport={handleExport}
      onImport={handleImport}
      onReset={handleReset}
    />
  )

  const debugPanel = (
    <PlaygroundDebugPanel
      debugData={debugData}
      activeDebugTab={activeDebugTab}
      customRequestMode={workbenchState.customRequestMode}
      onActiveDebugTabChange={setActiveDebugTab}
    />
  )

  return (
    <div className='relative flex size-full min-h-0 overflow-hidden'>
      {sessionSidebar}
      <div className='flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden'>
        <div className='border-border bg-background/80 flex h-12 shrink-0 items-center justify-between border-b px-3 backdrop-blur'>
          <div className='flex items-center gap-2'>
            <Button
              size='sm'
              variant='outline'
              className='inline-flex'
              onClick={() => setMobileSettingsOpen(true)}
            >
              <Settings2 className='mr-2 size-4' />
              {t('Settings')}
            </Button>
          </div>
          <Button
            size='sm'
            variant='outline'
            onClick={() => {
              if (isMobile) {
                setMobileDebugOpen(true)
                return
              }
              updateWorkbenchState(
                'showDebugPanel',
                !workbenchState.showDebugPanel
              )
            }}
          >
            <Bug className='mr-2 size-4' />
            {t('Debug')}
          </Button>
        </div>

        {sessionHeader}

        {/* Full-width scroll container: scrolling works even over side whitespace */}
        <div className='flex min-h-0 flex-1 flex-col overflow-hidden'>
          <PlaygroundChat
            messages={messages}
            onRegenerateMessage={handleRegenerateMessage}
            onEditMessage={handleEditMessage}
            onDeleteMessage={handleDeleteMessage}
            onSelectPrompt={handleSendMessage}
            isGenerating={isGenerating}
            editingKey={editingMessageKey}
            onCancelEdit={handleEditOpenChange}
            onSaveEdit={(newContent) => applyEdit(newContent, false)}
            onSaveEditAndSubmit={(newContent) => applyEdit(newContent, true)}
          />
        </div>

        {/* Input area: center content and constrain to the same container width */}
        <div className='mx-auto w-full max-w-4xl'>
          <PlaygroundInput
            disabled={isGenerating || isLoadingModels || models.length === 0}
            groups={groups}
            groupValue={config.group}
            isGenerating={isGenerating}
            isModelLoading={isLoadingModels || isLoadingGroups}
            modelValue={config.model}
            models={models}
            onGroupChange={(value) => updateConfig('group', value)}
            onModelChange={(value) => updateConfig('model', value)}
            onStop={stopGeneration}
            searchEnabled={workbenchState.searchEnabled}
            onSearchEnabledChange={(value) =>
              updateWorkbenchState('searchEnabled', value)
            }
            onSubmit={handleSendMessage}
            hasMessages={messages.length > 0}
            onClearMessages={handleClearChatHistory}
            showModelControls
          />
        </div>
      </div>

      {workbenchState.showDebugPanel && (
        <aside className='bg-background hidden w-[28rem] shrink-0 border-l xl:block'>
          {debugPanel}
        </aside>
      )}

      <div className='fixed right-4 bottom-4 z-40 flex flex-col gap-2 xl:hidden'>
        <Button
          size='icon'
          className={cn(
            'rounded-full shadow-lg',
            mobileSettingsOpen && 'hidden'
          )}
          onClick={() => setMobileSettingsOpen(true)}
          aria-label={t('Settings')}
        >
          <Settings2 className='size-5' />
        </Button>
        <Button
          size='icon'
          variant='secondary'
          className={cn('rounded-full shadow-lg', mobileDebugOpen && 'hidden')}
          onClick={() => setMobileDebugOpen(true)}
          aria-label={t('Debug')}
        >
          <Bug className='size-5' />
        </Button>
      </div>

      <Sheet open={mobileSettingsOpen} onOpenChange={setMobileSettingsOpen}>
        <SheetContent
          side='left'
          className='w-[96vw] gap-0 sm:max-w-2xl xl:max-w-5xl 2xl:max-w-6xl'
        >
          <SheetHeader className='shrink-0 border-b px-5 py-3'>
            <SheetTitle>{t('Playground Settings')}</SheetTitle>
            <SheetDescription>
              {t('Configure models, parameters, and custom request bodies.')}
            </SheetDescription>
          </SheetHeader>
          <div className='min-h-0 flex-1 overflow-hidden'>{settingsPanel}</div>
        </SheetContent>
      </Sheet>

      <Sheet open={mobileDebugOpen} onOpenChange={setMobileDebugOpen}>
        <SheetContent side='right' className='w-[92vw] sm:max-w-lg'>
          <SheetHeader>
            <SheetTitle>{t('Debug Information')}</SheetTitle>
            <SheetDescription>
              {t('Inspect preview, request, response, and raw SSE data.')}
            </SheetDescription>
          </SheetHeader>
          <div className='min-h-0 flex-1 overflow-hidden'>{debugPanel}</div>
        </SheetContent>
      </Sheet>
    </div>
  )
}
