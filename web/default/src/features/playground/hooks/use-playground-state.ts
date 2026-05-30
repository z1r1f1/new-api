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
import { useState, useCallback, useEffect } from 'react'
import {
  DEBUG_TABS,
  DEFAULT_CONFIG,
  DEFAULT_PARAMETER_ENABLED,
} from '../constants'
import {
  loadConfig,
  saveConfig,
  loadParameterEnabled,
  saveParameterEnabled,
  saveMessages,
  loadWorkbenchState,
  saveWorkbenchState,
  createPlaygroundSession,
  loadSessionState,
  saveActiveSessionId,
  saveSessions,
  updateStoredSessionMessages,
  getPlaygroundStorageScopeKey,
  PLAYGROUND_SESSION_MESSAGES_UPDATED_EVENT,
  type PlaygroundStorageScope,
  type PlaygroundSessionState,
} from '../lib'
import type {
  Message,
  PlaygroundConfig,
  ParameterEnabled,
  ModelOption,
  GroupOption,
  PlaygroundDebugData,
  PlaygroundDebugTab,
  PlaygroundSession,
  PlaygroundWorkbenchState,
} from '../types'

const initialDebugData: PlaygroundDebugData = {
  previewRequest: null,
  gatewayRequest: null,
  upstreamRequest: null,
  request: null,
  response: null,
  sseMessages: [],
  timestamp: null,
  previewTimestamp: null,
  isStreaming: false,
}

function getActiveSessionMessages(
  sessionState: PlaygroundSessionState
): Message[] {
  const activeSession =
    sessionState.sessions.find(
      (session) => session.id === sessionState.activeSessionId
    ) || sessionState.sessions[0]
  return activeSession?.messages || []
}

/**
 * Main state management hook for playground
 */
export function usePlaygroundState(storageUserId: PlaygroundStorageScope) {
  const [sessionState, setSessionState] = useState(() =>
    loadSessionState(storageUserId)
  )

  // Load initial state from localStorage
  const [config, setConfig] = useState<PlaygroundConfig>(() => {
    const savedConfig = loadConfig(storageUserId)
    return { ...DEFAULT_CONFIG, ...savedConfig }
  })

  const [parameterEnabled, setParameterEnabled] = useState<ParameterEnabled>(
    () => {
      const saved = loadParameterEnabled(storageUserId)
      return { ...DEFAULT_PARAMETER_ENABLED, ...saved }
    }
  )

  const [workbenchState, setWorkbenchState] =
    useState<PlaygroundWorkbenchState>(() => loadWorkbenchState(storageUserId))

  const [messages, setMessages] = useState<Message[]>(() => {
    return getActiveSessionMessages(sessionState)
  })

  const [models, setModels] = useState<ModelOption[]>([])
  const [groups, setGroups] = useState<GroupOption[]>([])
  const [debugData, setDebugData] =
    useState<PlaygroundDebugData>(initialDebugData)
  const [activeDebugTab, setActiveDebugTab] = useState<PlaygroundDebugTab>(
    DEBUG_TABS.PREVIEW
  )

  useEffect(() => {
    const scopeKey = getPlaygroundStorageScopeKey(storageUserId)
    const handleSessionMessagesUpdated = (event: Event) => {
      const detail = (event as CustomEvent<{ scope?: string }>).detail
      if (detail?.scope !== scopeKey) return

      const nextSessionState = loadSessionState(storageUserId)
      setSessionState(nextSessionState)
      setMessages(getActiveSessionMessages(nextSessionState))
    }

    window.addEventListener(
      PLAYGROUND_SESSION_MESSAGES_UPDATED_EVENT,
      handleSessionMessagesUpdated
    )
    return () =>
      window.removeEventListener(
        PLAYGROUND_SESSION_MESSAGES_UPDATED_EVENT,
        handleSessionMessagesUpdated
      )
  }, [storageUserId])

  // Update config with automatic save
  const updateConfig = useCallback(
    <K extends keyof PlaygroundConfig>(key: K, value: PlaygroundConfig[K]) => {
      setConfig((prev) => {
        const updated = { ...prev, [key]: value }
        saveConfig(updated, storageUserId)
        return updated
      })
    },
    [storageUserId]
  )

  // Update parameter enabled with automatic save
  const updateParameterEnabled = useCallback(
    (key: keyof ParameterEnabled, value: boolean) => {
      setParameterEnabled((prev) => {
        const updated = { ...prev, [key]: value }
        saveParameterEnabled(updated, storageUserId)
        return updated
      })
    },
    [storageUserId]
  )

  const updateWorkbenchState = useCallback(
    <K extends keyof PlaygroundWorkbenchState>(
      key: K,
      value: PlaygroundWorkbenchState[K]
    ) => {
      setWorkbenchState((prev) => {
        const updated = { ...prev, [key]: value }
        saveWorkbenchState(updated, storageUserId)
        return updated
      })
    },
    [storageUserId]
  )

  const replaceWorkbenchState = useCallback(
    (next: Partial<PlaygroundWorkbenchState>) => {
      setWorkbenchState((prev) => {
        const updated = { ...prev, ...next }
        saveWorkbenchState(updated, storageUserId)
        return updated
      })
    },
    [storageUserId]
  )

  const updateSessionMessages = useCallback(
    (
      sessionId: string,
      updater: Message[] | ((prev: Message[]) => Message[])
    ) => {
      const result = updateStoredSessionMessages(
        sessionId,
        updater,
        storageUserId
      )
      if (!result.updated || !result.messages) return

      setSessionState({
        sessions: result.sessions,
        activeSessionId: result.activeSessionId,
      })
      if (result.activeSessionId === sessionId) {
        setMessages(result.messages)
      }
    },
    [storageUserId]
  )

  // Update active-session messages with automatic save
  const updateMessages = useCallback(
    (updater: Message[] | ((prev: Message[]) => Message[])) => {
      updateSessionMessages(
        loadSessionState(storageUserId).activeSessionId,
        updater
      )
    },
    [storageUserId, updateSessionMessages]
  )

  // Clear all messages
  const clearMessages = useCallback(() => {
    updateMessages([])
  }, [updateMessages])

  // Reset config to defaults
  const resetConfig = useCallback(() => {
    setConfig(DEFAULT_CONFIG)
    setParameterEnabled(DEFAULT_PARAMETER_ENABLED)
    saveConfig(DEFAULT_CONFIG, storageUserId)
    saveParameterEnabled(DEFAULT_PARAMETER_ENABLED, storageUserId)
  }, [storageUserId])

  const replaceConfig = useCallback(
    (next: Partial<PlaygroundConfig>) => {
      setConfig((prev) => {
        const updated = { ...prev, ...next }
        saveConfig(updated, storageUserId)
        return updated
      })
    },
    [storageUserId]
  )

  const replaceParameterEnabled = useCallback(
    (next: Partial<ParameterEnabled>) => {
      setParameterEnabled((prev) => {
        const updated = { ...prev, ...next }
        saveParameterEnabled(updated, storageUserId)
        return updated
      })
    },
    [storageUserId]
  )

  const resetDebugData = useCallback(() => {
    setDebugData(initialDebugData)
    setActiveDebugTab(DEBUG_TABS.PREVIEW)
  }, [])

  const switchSession = useCallback(
    (sessionId: string) => {
      setSessionState((prev) => {
        const targetSession = prev.sessions.find(
          (session) => session.id === sessionId
        )
        if (!targetSession) return prev

        saveActiveSessionId(sessionId, storageUserId)
        saveMessages(targetSession.messages, storageUserId)
        setMessages(targetSession.messages)
        setDebugData(initialDebugData)
        setActiveDebugTab(DEBUG_TABS.PREVIEW)
        return {
          ...prev,
          activeSessionId: sessionId,
        }
      })
    },
    [storageUserId]
  )

  const createSession = useCallback(() => {
    const nextSession = createPlaygroundSession()
    setSessionState((prev) => {
      const sessions = [nextSession, ...prev.sessions]
      saveSessions(sessions, storageUserId)
      saveActiveSessionId(nextSession.id, storageUserId)
      return {
        sessions,
        activeSessionId: nextSession.id,
      }
    })
    setMessages([])
    saveMessages([], storageUserId)
    setDebugData(initialDebugData)
    setActiveDebugTab(DEBUG_TABS.PREVIEW)
  }, [storageUserId])

  const renameSession = useCallback(
    (sessionId: string, title: string) => {
      const trimmedTitle = title.trim()
      if (!trimmedTitle) return

      setSessionState((prev) => {
        const sessions = prev.sessions.map((session) =>
          session.id === sessionId
            ? {
                ...session,
                title: trimmedTitle,
                updatedAt: new Date().toISOString(),
              }
            : session
        )
        saveSessions(sessions, storageUserId)
        return { ...prev, sessions }
      })
    },
    [storageUserId]
  )

  const deleteSession = useCallback(
    (sessionId: string) => {
      setSessionState((prev) => {
        const remainingSessions = prev.sessions.filter(
          (session) => session.id !== sessionId
        )
        const sessions =
          remainingSessions.length > 0
            ? remainingSessions
            : [createPlaygroundSession()]
        const activeSession =
          prev.activeSessionId === sessionId
            ? sessions[0]
            : sessions.find((session) => session.id === prev.activeSessionId) ||
              sessions[0]

        saveSessions(sessions, storageUserId)
        saveActiveSessionId(activeSession.id, storageUserId)
        setMessages(activeSession.messages)
        saveMessages(activeSession.messages, storageUserId)
        setDebugData(initialDebugData)
        setActiveDebugTab(DEBUG_TABS.PREVIEW)

        return {
          sessions,
          activeSessionId: activeSession.id,
        }
      })
    },
    [storageUserId]
  )

  const replaceSessions = useCallback(
    (sessions: PlaygroundSession[], activeSessionId?: string) => {
      if (sessions.length === 0) return

      const activeSession =
        sessions.find((session) => session.id === activeSessionId) ||
        sessions[0]

      setSessionState({
        sessions,
        activeSessionId: activeSession.id,
      })
      saveSessions(sessions, storageUserId)
      saveActiveSessionId(activeSession.id, storageUserId)
      setMessages(activeSession.messages)
      saveMessages(activeSession.messages, storageUserId)
      setDebugData(initialDebugData)
      setActiveDebugTab(DEBUG_TABS.PREVIEW)
    },
    [storageUserId]
  )

  return {
    // State
    storageUserId: storageUserId as PlaygroundStorageScope,
    config,
    parameterEnabled,
    workbenchState,
    messages,
    sessions: sessionState.sessions,
    activeSessionId: sessionState.activeSessionId,
    models,
    groups,
    debugData,
    activeDebugTab,

    // Setters
    setModels,
    setGroups,
    setDebugData,
    setActiveDebugTab,

    // Actions
    updateConfig,
    updateParameterEnabled,
    updateWorkbenchState,
    replaceWorkbenchState,
    updateMessages,
    updateSessionMessages,
    clearMessages,
    resetConfig,
    replaceConfig,
    replaceParameterEnabled,
    resetDebugData,
    switchSession,
    createSession,
    renameSession,
    deleteSession,
    replaceSessions,
  }
}
