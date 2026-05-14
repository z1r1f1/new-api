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
import { STORAGE_KEYS } from '../constants'
import type {
  Message,
  ParameterEnabled,
  PlaygroundConfig,
  PlaygroundImportData,
  PlaygroundSession,
  PlaygroundWorkbenchState,
} from '../types'
import { getCurrentVersion, sanitizeMessagesOnLoad } from './message-utils'

const defaultWorkbenchState: PlaygroundWorkbenchState = {
  showSettings: true,
  showDebugPanel: false,
  customRequestMode: false,
  customRequestBody: '',
}

export interface PlaygroundSessionState {
  sessions: PlaygroundSession[]
  activeSessionId: string
}

const maxStoredSessions = 30

function parseJSON(value: string | null): unknown {
  if (!value) return null
  return JSON.parse(value)
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function generateSessionId(): string {
  if (typeof crypto !== 'undefined' && crypto.randomUUID) {
    return crypto.randomUUID()
  }
  return `session-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
}

export function buildSessionTitle(messages: Message[]): string {
  const firstUserMessage = messages.find((message) => message.from === 'user')
  const content = firstUserMessage
    ? getCurrentVersion(firstUserMessage).content.trim()
    : ''

  if (!content) return 'New session'
  return content.length > 28 ? `${content.slice(0, 28)}...` : content
}

function normalizeSession(value: unknown): PlaygroundSession | null {
  if (!isRecord(value)) return null

  const messages = Array.isArray(value.messages)
    ? sanitizeMessagesOnLoad(value.messages as Message[])
    : []
  const now = new Date().toISOString()
  const id =
    typeof value.id === 'string' && value.id.trim()
      ? value.id
      : generateSessionId()

  return {
    id,
    title:
      typeof value.title === 'string' && value.title.trim()
        ? value.title.trim()
        : buildSessionTitle(messages),
    messages,
    createdAt:
      typeof value.createdAt === 'string' && value.createdAt
        ? value.createdAt
        : now,
    updatedAt:
      typeof value.updatedAt === 'string' && value.updatedAt
        ? value.updatedAt
        : now,
  }
}

function trimSessions(sessions: PlaygroundSession[]): PlaygroundSession[] {
  return [...sessions]
    .sort((a, b) => b.updatedAt.localeCompare(a.updatedAt))
    .slice(0, maxStoredSessions)
}

function createSession(messages: Message[] = []): PlaygroundSession {
  const now = new Date().toISOString()
  return {
    id: generateSessionId(),
    title: buildSessionTitle(messages),
    messages,
    createdAt: now,
    updatedAt: now,
  }
}

function normalizeConfig(value: unknown): Partial<PlaygroundConfig> {
  if (!isRecord(value)) return {}

  // Classic Playground stored API fields under `inputs`; support that shape so
  // users switching from classic/default do not lose their latest test config.
  const candidate = isRecord(value.inputs) ? value.inputs : value
  const result: Partial<PlaygroundConfig> = {}

  if (typeof candidate.model === 'string') result.model = candidate.model
  if (typeof candidate.group === 'string') result.group = candidate.group
  if (typeof candidate.temperature === 'number') {
    result.temperature = candidate.temperature
  }
  if (typeof candidate.top_p === 'number') result.top_p = candidate.top_p
  if (typeof candidate.max_tokens === 'number') {
    result.max_tokens = candidate.max_tokens
  }
  if (typeof candidate.frequency_penalty === 'number') {
    result.frequency_penalty = candidate.frequency_penalty
  }
  if (typeof candidate.presence_penalty === 'number') {
    result.presence_penalty = candidate.presence_penalty
  }
  if (typeof candidate.seed === 'number' || candidate.seed === null) {
    result.seed = candidate.seed
  }
  if (typeof candidate.stream === 'boolean') result.stream = candidate.stream

  return result
}

function normalizeParameterEnabled(
  value: unknown
): Partial<ParameterEnabled> {
  if (!isRecord(value)) return {}
  const result: Partial<ParameterEnabled> = {}
  const keys: Array<keyof ParameterEnabled> = [
    'temperature',
    'top_p',
    'max_tokens',
    'frequency_penalty',
    'presence_penalty',
    'seed',
  ]

  keys.forEach((key) => {
    if (typeof value[key] === 'boolean') {
      result[key] = value[key]
    }
  })

  return result
}

function normalizeWorkbenchState(value: unknown): Partial<PlaygroundWorkbenchState> {
  if (!isRecord(value)) return {}
  const result: Partial<PlaygroundWorkbenchState> = {}

  if (typeof value.showSettings === 'boolean') {
    result.showSettings = value.showSettings
  }
  if (typeof value.showDebugPanel === 'boolean') {
    result.showDebugPanel = value.showDebugPanel
  }
  if (typeof value.customRequestMode === 'boolean') {
    result.customRequestMode = value.customRequestMode
  }
  if (typeof value.customRequestBody === 'string') {
    result.customRequestBody = value.customRequestBody
  }

  return result
}

/**
 * Load playground config from localStorage
 */
export function loadConfig(): Partial<PlaygroundConfig> {
  try {
    return normalizeConfig(parseJSON(localStorage.getItem(STORAGE_KEYS.CONFIG)))
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to load config:', error)
  }
  return {}
}

/**
 * Save playground config to localStorage
 */
export function saveConfig(config: Partial<PlaygroundConfig>): void {
  try {
    localStorage.setItem(
      STORAGE_KEYS.CONFIG,
      JSON.stringify({ ...config, timestamp: new Date().toISOString() })
    )
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to save config:', error)
  }
}

/**
 * Load parameter enabled state from localStorage
 */
export function loadParameterEnabled(): Partial<ParameterEnabled> {
  try {
    return normalizeParameterEnabled(
      parseJSON(localStorage.getItem(STORAGE_KEYS.PARAMETER_ENABLED))
    )
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to load parameter enabled:', error)
  }
  return {}
}

/**
 * Save parameter enabled state to localStorage
 */
export function saveParameterEnabled(
  parameterEnabled: Partial<ParameterEnabled>
): void {
  try {
    localStorage.setItem(
      STORAGE_KEYS.PARAMETER_ENABLED,
      JSON.stringify(parameterEnabled)
    )
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to save parameter enabled:', error)
  }
}

export function loadWorkbenchState(): PlaygroundWorkbenchState {
  try {
    const saved = normalizeWorkbenchState(
      parseJSON(localStorage.getItem(STORAGE_KEYS.WORKBENCH))
    )
    const legacyConfig = parseJSON(localStorage.getItem(STORAGE_KEYS.CONFIG))
    const legacyWorkbench = normalizeWorkbenchState(legacyConfig)
    return { ...defaultWorkbenchState, ...legacyWorkbench, ...saved }
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to load playground workbench state:', error)
  }
  return defaultWorkbenchState
}

export function saveWorkbenchState(
  workbenchState: Partial<PlaygroundWorkbenchState>
): void {
  try {
    localStorage.setItem(
      STORAGE_KEYS.WORKBENCH,
      JSON.stringify({
        ...workbenchState,
        timestamp: new Date().toISOString(),
      })
    )
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to save playground workbench state:', error)
  }
}

export function loadSessions(): PlaygroundSession[] {
  try {
    const saved = parseJSON(localStorage.getItem(STORAGE_KEYS.SESSIONS))
    const savedSessions = Array.isArray(saved)
      ? saved
          .map(normalizeSession)
          .filter((session): session is PlaygroundSession => session !== null)
      : []

    if (savedSessions.length > 0) {
      return trimSessions(savedSessions)
    }

    const legacyMessages = loadMessages()
    return [createSession(legacyMessages || [])]
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to load playground sessions:', error)
  }

  return [createSession()]
}

export function saveSessions(sessions: PlaygroundSession[]): void {
  try {
    localStorage.setItem(
      STORAGE_KEYS.SESSIONS,
      JSON.stringify(trimSessions(sessions))
    )
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to save playground sessions:', error)
  }
}

export function saveActiveSessionId(sessionId: string): void {
  try {
    localStorage.setItem(STORAGE_KEYS.ACTIVE_SESSION_ID, sessionId)
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to save active playground session:', error)
  }
}

export function loadSessionState(): PlaygroundSessionState {
  const sessions = loadSessions()
  const storedActiveId = localStorage.getItem(STORAGE_KEYS.ACTIVE_SESSION_ID)
  const activeSession =
    sessions.find((session) => session.id === storedActiveId) || sessions[0]

  saveSessions(sessions)
  saveActiveSessionId(activeSession.id)

  return {
    sessions,
    activeSessionId: activeSession.id,
  }
}

export function createPlaygroundSession(): PlaygroundSession {
  return createSession()
}

/**
 * Load messages from localStorage
 */
export function loadMessages(): Message[] | null {
  try {
    const saved = localStorage.getItem(STORAGE_KEYS.MESSAGES)
    if (saved) {
      const parsed = parseJSON(saved)
      const messages = Array.isArray(parsed)
        ? parsed
        : isRecord(parsed) && Array.isArray(parsed.messages)
          ? parsed.messages
          : null
      if (!messages) {
        localStorage.removeItem(STORAGE_KEYS.MESSAGES)
        return null
      }
      const sanitized = sanitizeMessagesOnLoad(messages as Message[])
      // Persist sanitized result to avoid re-sanitizing legacy shapes on subsequent loads
      saveMessages(sanitized)
      return sanitized
    }
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to load messages:', error)
  }
  return null
}

/**
 * Save messages to localStorage
 */
export function saveMessages(messages: Message[]): void {
  try {
    localStorage.setItem(STORAGE_KEYS.MESSAGES, JSON.stringify(messages))
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to save messages:', error)
  }
}

/**
 * Clear all playground data
 */
export function clearPlaygroundData(): void {
  try {
    localStorage.removeItem(STORAGE_KEYS.CONFIG)
    localStorage.removeItem(STORAGE_KEYS.PARAMETER_ENABLED)
    localStorage.removeItem(STORAGE_KEYS.MESSAGES)
    localStorage.removeItem(STORAGE_KEYS.SESSIONS)
    localStorage.removeItem(STORAGE_KEYS.ACTIVE_SESSION_ID)
    localStorage.removeItem(STORAGE_KEYS.WORKBENCH)
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to clear playground data:', error)
  }
}

export function exportPlaygroundData(params: {
  config: PlaygroundConfig
  parameterEnabled: ParameterEnabled
  workbenchState: PlaygroundWorkbenchState
  messages: Message[]
  sessions: PlaygroundSession[]
  activeSessionId: string
}): void {
  const payload = {
    config: params.config,
    parameterEnabled: params.parameterEnabled,
    customRequestMode: params.workbenchState.customRequestMode,
    customRequestBody: params.workbenchState.customRequestBody,
    showDebugPanel: params.workbenchState.showDebugPanel,
    messages: params.messages,
    sessions: params.sessions,
    activeSessionId: params.activeSessionId,
    exportTime: new Date().toISOString(),
    version: 'default-playground-v1',
  }
  const blob = new Blob([JSON.stringify(payload, null, 2)], {
    type: 'application/json',
  })
  const link = document.createElement('a')
  link.href = URL.createObjectURL(blob)
  link.download = `playground-config-${new Date().toISOString().slice(0, 10)}.json`
  link.click()
  URL.revokeObjectURL(link.href)
}

export function importPlaygroundData(file: File): Promise<PlaygroundImportData> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => {
      try {
        const parsed = parseJSON(String(reader.result || ''))
        if (!isRecord(parsed)) {
          reject(new Error('Invalid playground config file'))
          return
        }
        resolve({
          config: normalizeConfig(parsed.config || parsed.inputs || parsed),
          inputs: normalizeConfig(parsed.inputs),
          parameterEnabled: normalizeParameterEnabled(parsed.parameterEnabled),
          ...normalizeWorkbenchState(parsed),
          messages: Array.isArray(parsed.messages)
            ? sanitizeMessagesOnLoad(parsed.messages as Message[])
            : undefined,
          sessions: Array.isArray(parsed.sessions)
            ? parsed.sessions
                .map(normalizeSession)
                .filter(
                  (session): session is PlaygroundSession => session !== null
                )
            : undefined,
          activeSessionId:
            typeof parsed.activeSessionId === 'string'
              ? parsed.activeSessionId
              : undefined,
        })
      } catch (error) {
        reject(error instanceof Error ? error : new Error(String(error)))
      }
    }
    reader.onerror = () => reject(new Error('Failed to read file'))
    reader.readAsText(file)
  })
}
