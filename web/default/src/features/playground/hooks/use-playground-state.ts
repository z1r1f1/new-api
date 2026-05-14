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
import { useState, useCallback } from 'react'
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
  buildSessionTitle,
  createPlaygroundSession,
  loadSessionState,
  saveActiveSessionId,
  saveSessions,
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

/**
 * Main state management hook for playground
 */
export function usePlaygroundState() {
  const [sessionState, setSessionState] = useState(() => loadSessionState())

  // Load initial state from localStorage
  const [config, setConfig] = useState<PlaygroundConfig>(() => {
    const savedConfig = loadConfig()
    return { ...DEFAULT_CONFIG, ...savedConfig }
  })

  const [parameterEnabled, setParameterEnabled] = useState<ParameterEnabled>(
    () => {
      const saved = loadParameterEnabled()
      return { ...DEFAULT_PARAMETER_ENABLED, ...saved }
    }
  )

  const [workbenchState, setWorkbenchState] =
    useState<PlaygroundWorkbenchState>(() => loadWorkbenchState())

  const [messages, setMessages] = useState<Message[]>(() => {
    const activeSession =
      sessionState.sessions.find(
        (session) => session.id === sessionState.activeSessionId
      ) || sessionState.sessions[0]
    return activeSession?.messages || []
  })

  const [models, setModels] = useState<ModelOption[]>([])
  const [groups, setGroups] = useState<GroupOption[]>([])
  const [debugData, setDebugData] =
    useState<PlaygroundDebugData>(initialDebugData)
  const [activeDebugTab, setActiveDebugTab] = useState<PlaygroundDebugTab>(
    DEBUG_TABS.PREVIEW
  )

  // Update config with automatic save
  const updateConfig = useCallback(
    <K extends keyof PlaygroundConfig>(key: K, value: PlaygroundConfig[K]) => {
      setConfig((prev) => {
        const updated = { ...prev, [key]: value }
        saveConfig(updated)
        return updated
      })
    },
    []
  )

  // Update parameter enabled with automatic save
  const updateParameterEnabled = useCallback(
    (key: keyof ParameterEnabled, value: boolean) => {
      setParameterEnabled((prev) => {
        const updated = { ...prev, [key]: value }
        saveParameterEnabled(updated)
        return updated
      })
    },
    []
  )

  const updateWorkbenchState = useCallback(
    <K extends keyof PlaygroundWorkbenchState>(
      key: K,
      value: PlaygroundWorkbenchState[K]
    ) => {
      setWorkbenchState((prev) => {
        const updated = { ...prev, [key]: value }
        saveWorkbenchState(updated)
        return updated
      })
    },
    []
  )

  const replaceWorkbenchState = useCallback(
    (next: Partial<PlaygroundWorkbenchState>) => {
      setWorkbenchState((prev) => {
        const updated = { ...prev, ...next }
        saveWorkbenchState(updated)
        return updated
      })
    },
    []
  )

  // Update messages with automatic save
  const updateMessages = useCallback(
    (updater: Message[] | ((prev: Message[]) => Message[])) => {
      setMessages((prev) => {
        const newMessages =
          typeof updater === 'function' ? updater(prev) : updater
        setSessionState((prevSessionState) => {
          const now = new Date().toISOString()
          const updatedSessions = prevSessionState.sessions.map((session) => {
            if (session.id !== prevSessionState.activeSessionId) return session

            return {
              ...session,
              title:
                session.title && session.title !== 'New session'
                  ? session.title
                  : buildSessionTitle(newMessages),
              messages: newMessages,
              updatedAt: now,
            }
          })
          saveSessions(updatedSessions)
          return {
            ...prevSessionState,
            sessions: updatedSessions,
          }
        })
        saveMessages(newMessages)
        return newMessages
      })
    },
    []
  )

  // Clear all messages
  const clearMessages = useCallback(() => {
    updateMessages([])
  }, [updateMessages])

  // Reset config to defaults
  const resetConfig = useCallback(() => {
    setConfig(DEFAULT_CONFIG)
    setParameterEnabled(DEFAULT_PARAMETER_ENABLED)
    saveConfig(DEFAULT_CONFIG)
    saveParameterEnabled(DEFAULT_PARAMETER_ENABLED)
  }, [])

  const replaceConfig = useCallback((next: Partial<PlaygroundConfig>) => {
    setConfig((prev) => {
      const updated = { ...prev, ...next }
      saveConfig(updated)
      return updated
    })
  }, [])

  const replaceParameterEnabled = useCallback(
    (next: Partial<ParameterEnabled>) => {
      setParameterEnabled((prev) => {
        const updated = { ...prev, ...next }
        saveParameterEnabled(updated)
        return updated
      })
    },
    []
  )

  const resetDebugData = useCallback(() => {
    setDebugData(initialDebugData)
    setActiveDebugTab(DEBUG_TABS.PREVIEW)
  }, [])

  const switchSession = useCallback((sessionId: string) => {
    setSessionState((prev) => {
      const targetSession = prev.sessions.find(
        (session) => session.id === sessionId
      )
      if (!targetSession) return prev

      saveActiveSessionId(sessionId)
      saveMessages(targetSession.messages)
      setMessages(targetSession.messages)
      setDebugData(initialDebugData)
      setActiveDebugTab(DEBUG_TABS.PREVIEW)
      return {
        ...prev,
        activeSessionId: sessionId,
      }
    })
  }, [])

  const createSession = useCallback(() => {
    const nextSession = createPlaygroundSession()
    setSessionState((prev) => {
      const sessions = [nextSession, ...prev.sessions]
      saveSessions(sessions)
      saveActiveSessionId(nextSession.id)
      return {
        sessions,
        activeSessionId: nextSession.id,
      }
    })
    setMessages([])
    saveMessages([])
    setDebugData(initialDebugData)
    setActiveDebugTab(DEBUG_TABS.PREVIEW)
  }, [])

  const renameSession = useCallback((sessionId: string, title: string) => {
    const trimmedTitle = title.trim()
    if (!trimmedTitle) return

    setSessionState((prev) => {
      const sessions = prev.sessions.map((session) =>
        session.id === sessionId
          ? { ...session, title: trimmedTitle, updatedAt: new Date().toISOString() }
          : session
      )
      saveSessions(sessions)
      return { ...prev, sessions }
    })
  }, [])

  const deleteSession = useCallback((sessionId: string) => {
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

      saveSessions(sessions)
      saveActiveSessionId(activeSession.id)
      setMessages(activeSession.messages)
      saveMessages(activeSession.messages)
      setDebugData(initialDebugData)
      setActiveDebugTab(DEBUG_TABS.PREVIEW)

      return {
        sessions,
        activeSessionId: activeSession.id,
      }
    })
  }, [])

  const replaceSessions = useCallback(
    (sessions: PlaygroundSession[], activeSessionId?: string) => {
      if (sessions.length === 0) return

      const activeSession =
        sessions.find((session) => session.id === activeSessionId) || sessions[0]

      setSessionState({
        sessions,
        activeSessionId: activeSession.id,
      })
      saveSessions(sessions)
      saveActiveSessionId(activeSession.id)
      setMessages(activeSession.messages)
      saveMessages(activeSession.messages)
      setDebugData(initialDebugData)
      setActiveDebugTab(DEBUG_TABS.PREVIEW)
    },
    []
  )

  return {
    // State
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
