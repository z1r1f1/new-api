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
  PendingImageGenerationTask,
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
  searchEnabled: false,
}

export interface PlaygroundSessionState {
  sessions: PlaygroundSession[]
  activeSessionId: string
}

export type PlaygroundStorageScope = string | number | null | undefined

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

function normalizeStorageScope(scope: PlaygroundStorageScope): string | null {
  if (scope === undefined) return null
  if (scope === null || scope === '') return 'anonymous'
  return `user:${String(scope)}`
}

function getStorageKey(key: string, scope?: PlaygroundStorageScope): string {
  const normalizedScope = normalizeStorageScope(scope)
  return normalizedScope ? `${key}:${normalizedScope}` : key
}

function removeLegacyStorageKey(
  key: string,
  scope?: PlaygroundStorageScope
): void {
  if (normalizeStorageScope(scope) !== null) {
    localStorage.removeItem(key)
  }
}

function getStorageItem(
  key: string,
  scope?: PlaygroundStorageScope
): string | null {
  const storageKey = getStorageKey(key, scope)

  if (storageKey === key) {
    return localStorage.getItem(key)
  }

  const scopedValue = localStorage.getItem(storageKey)
  if (scopedValue !== null) {
    removeLegacyStorageKey(key, scope)
    return scopedValue
  }

  const legacyValue = localStorage.getItem(key)
  if (legacyValue !== null) {
    localStorage.setItem(storageKey, legacyValue)
    removeLegacyStorageKey(key, scope)
    return legacyValue
  }

  return null
}

function setStorageItem(
  key: string,
  value: string,
  scope?: PlaygroundStorageScope
): void {
  localStorage.setItem(getStorageKey(key, scope), value)
  removeLegacyStorageKey(key, scope)
}

function removeStorageItem(key: string, scope?: PlaygroundStorageScope): void {
  const normalizedScope = normalizeStorageScope(scope)

  if (normalizedScope !== null) {
    localStorage.removeItem(getStorageKey(key, scope))
    localStorage.removeItem(key)
    return
  }

  localStorage.removeItem(key)
  for (let index = localStorage.length - 1; index >= 0; index -= 1) {
    const storageKey = localStorage.key(index)
    if (storageKey?.startsWith(`${key}:`)) {
      localStorage.removeItem(storageKey)
    }
  }
}

export function buildSessionTitle(messages: Message[]): string {
  const firstUserMessage = messages.find((message) => message.from === 'user')
  const content = firstUserMessage
    ? getCurrentVersion(firstUserMessage).content.trim()
    : ''

  if (!content) return 'New session'
  return content.length > 28 ? `${content.slice(0, 28)}...` : content
}

function loadPendingImageMessageKeys(scope?: PlaygroundStorageScope): string[] {
  return loadPendingImageTasks(scope).map((task) => task.messageKey)
}

function normalizeSession(
  value: unknown,
  scope?: PlaygroundStorageScope
): PlaygroundSession | null {
  if (!isRecord(value)) return null

  const pendingMessageKeys = loadPendingImageMessageKeys(scope)
  const messages = Array.isArray(value.messages)
    ? sanitizeMessagesOnLoad(value.messages as Message[], pendingMessageKeys)
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
  if (typeof candidate.deep_research === 'boolean') {
    result.deep_research = candidate.deep_research
  }

  return result
}

function normalizeParameterEnabled(value: unknown): Partial<ParameterEnabled> {
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

function normalizeWorkbenchState(
  value: unknown
): Partial<PlaygroundWorkbenchState> {
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
  if (typeof value.searchEnabled === 'boolean') {
    result.searchEnabled = value.searchEnabled
  }

  return result
}

function normalizePendingImageTask(
  value: unknown
): PendingImageGenerationTask | null {
  if (!isRecord(value)) return null

  const taskId = typeof value.taskId === 'string' ? value.taskId.trim() : ''
  const messageKey =
    typeof value.messageKey === 'string' ? value.messageKey.trim() : ''
  const sessionId =
    typeof value.sessionId === 'string' ? value.sessionId.trim() : ''
  const startedAt =
    typeof value.startedAt === 'number' && Number.isFinite(value.startedAt)
      ? value.startedAt
      : 0

  if (!taskId || !messageKey || !sessionId || startedAt <= 0) {
    return null
  }

  return {
    taskId,
    messageKey,
    sessionId,
    debugId: typeof value.debugId === 'string' ? value.debugId.trim() : '',
    startedAt,
    updatedAt:
      typeof value.updatedAt === 'string' && value.updatedAt
        ? value.updatedAt
        : new Date().toISOString(),
  }
}

/**
 * Load playground config from localStorage
 */
export function loadConfig(
  scope?: PlaygroundStorageScope
): Partial<PlaygroundConfig> {
  try {
    return normalizeConfig(
      parseJSON(getStorageItem(STORAGE_KEYS.CONFIG, scope))
    )
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to load config:', error)
  }
  return {}
}

/**
 * Save playground config to localStorage
 */
export function saveConfig(
  config: Partial<PlaygroundConfig>,
  scope?: PlaygroundStorageScope
): void {
  try {
    setStorageItem(
      STORAGE_KEYS.CONFIG,
      JSON.stringify({ ...config, timestamp: new Date().toISOString() }),
      scope
    )
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to save config:', error)
  }
}

/**
 * Load parameter enabled state from localStorage
 */
export function loadParameterEnabled(
  scope?: PlaygroundStorageScope
): Partial<ParameterEnabled> {
  try {
    return normalizeParameterEnabled(
      parseJSON(getStorageItem(STORAGE_KEYS.PARAMETER_ENABLED, scope))
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
  parameterEnabled: Partial<ParameterEnabled>,
  scope?: PlaygroundStorageScope
): void {
  try {
    setStorageItem(
      STORAGE_KEYS.PARAMETER_ENABLED,
      JSON.stringify(parameterEnabled),
      scope
    )
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to save parameter enabled:', error)
  }
}

export function loadWorkbenchState(
  scope?: PlaygroundStorageScope
): PlaygroundWorkbenchState {
  try {
    const saved = normalizeWorkbenchState(
      parseJSON(getStorageItem(STORAGE_KEYS.WORKBENCH, scope))
    )
    const legacyConfig = parseJSON(getStorageItem(STORAGE_KEYS.CONFIG, scope))
    const legacyWorkbench = normalizeWorkbenchState(legacyConfig)
    return { ...defaultWorkbenchState, ...legacyWorkbench, ...saved }
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to load playground workbench state:', error)
  }
  return defaultWorkbenchState
}

export function saveWorkbenchState(
  workbenchState: Partial<PlaygroundWorkbenchState>,
  scope?: PlaygroundStorageScope
): void {
  try {
    setStorageItem(
      STORAGE_KEYS.WORKBENCH,
      JSON.stringify({
        ...workbenchState,
        timestamp: new Date().toISOString(),
      }),
      scope
    )
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to save playground workbench state:', error)
  }
}

export function loadSessions(
  scope?: PlaygroundStorageScope
): PlaygroundSession[] {
  try {
    const saved = parseJSON(getStorageItem(STORAGE_KEYS.SESSIONS, scope))
    const savedSessions = Array.isArray(saved)
      ? saved
          .map((session) => normalizeSession(session, scope))
          .filter((session): session is PlaygroundSession => session !== null)
      : []

    if (savedSessions.length > 0) {
      return trimSessions(savedSessions)
    }

    const legacyMessages = loadMessages(scope)
    return [createSession(legacyMessages || [])]
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to load playground sessions:', error)
  }

  return [createSession()]
}

export function saveSessions(
  sessions: PlaygroundSession[],
  scope?: PlaygroundStorageScope
): void {
  try {
    setStorageItem(
      STORAGE_KEYS.SESSIONS,
      JSON.stringify(trimSessions(sessions)),
      scope
    )
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to save playground sessions:', error)
  }
}

export function saveActiveSessionId(
  sessionId: string,
  scope?: PlaygroundStorageScope
): void {
  try {
    setStorageItem(STORAGE_KEYS.ACTIVE_SESSION_ID, sessionId, scope)
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to save active playground session:', error)
  }
}

export function loadSessionState(
  scope?: PlaygroundStorageScope
): PlaygroundSessionState {
  const sessions = loadSessions(scope)
  const storedActiveId = getStorageItem(STORAGE_KEYS.ACTIVE_SESSION_ID, scope)
  const activeSession =
    sessions.find((session) => session.id === storedActiveId) || sessions[0]

  saveSessions(sessions, scope)
  saveActiveSessionId(activeSession.id, scope)

  return {
    sessions,
    activeSessionId: activeSession.id,
  }
}

export function createPlaygroundSession(): PlaygroundSession {
  return createSession()
}

export function loadPendingImageTasks(
  scope?: PlaygroundStorageScope
): PendingImageGenerationTask[] {
  try {
    const saved = parseJSON(
      getStorageItem(STORAGE_KEYS.PENDING_IMAGE_TASKS, scope)
    )
    if (!Array.isArray(saved)) {
      return []
    }
    return saved
      .map(normalizePendingImageTask)
      .filter((task): task is PendingImageGenerationTask => task !== null)
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to load pending playground image tasks:', error)
  }
  return []
}

export function savePendingImageTasks(
  tasks: PendingImageGenerationTask[],
  scope?: PlaygroundStorageScope
): void {
  try {
    setStorageItem(
      STORAGE_KEYS.PENDING_IMAGE_TASKS,
      JSON.stringify(tasks),
      scope
    )
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to save pending playground image tasks:', error)
  }
}

export function upsertPendingImageTask(
  task: PendingImageGenerationTask,
  scope?: PlaygroundStorageScope
): void {
  const tasks = loadPendingImageTasks(scope).filter(
    (item) => item.taskId !== task.taskId
  )
  savePendingImageTasks(
    [
      ...tasks,
      {
        ...task,
        updatedAt: new Date().toISOString(),
      },
    ],
    scope
  )
}

export function removePendingImageTask(
  taskId: string,
  scope?: PlaygroundStorageScope
): void {
  const trimmedTaskId = taskId.trim()
  if (!trimmedTaskId) return

  savePendingImageTasks(
    loadPendingImageTasks(scope).filter(
      (task) => task.taskId !== trimmedTaskId
    ),
    scope
  )
}

/**
 * Load messages from localStorage
 */
export function loadMessages(scope?: PlaygroundStorageScope): Message[] | null {
  try {
    const saved = getStorageItem(STORAGE_KEYS.MESSAGES, scope)
    if (saved) {
      const parsed = parseJSON(saved)
      const messages = Array.isArray(parsed)
        ? parsed
        : isRecord(parsed) && Array.isArray(parsed.messages)
          ? parsed.messages
          : null
      if (!messages) {
        removeStorageItem(STORAGE_KEYS.MESSAGES, scope)
        return null
      }
      const sanitized = sanitizeMessagesOnLoad(
        messages as Message[],
        loadPendingImageMessageKeys(scope)
      )
      // Persist sanitized result to avoid re-sanitizing legacy shapes on subsequent loads
      saveMessages(sanitized, scope)
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
export function saveMessages(
  messages: Message[],
  scope?: PlaygroundStorageScope
): void {
  try {
    setStorageItem(STORAGE_KEYS.MESSAGES, JSON.stringify(messages), scope)
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to save messages:', error)
  }
}

/**
 * Clear all playground data
 */
export function clearPlaygroundData(scope?: PlaygroundStorageScope): void {
  try {
    removeStorageItem(STORAGE_KEYS.CONFIG, scope)
    removeStorageItem(STORAGE_KEYS.PARAMETER_ENABLED, scope)
    removeStorageItem(STORAGE_KEYS.MESSAGES, scope)
    removeStorageItem(STORAGE_KEYS.SESSIONS, scope)
    removeStorageItem(STORAGE_KEYS.ACTIVE_SESSION_ID, scope)
    removeStorageItem(STORAGE_KEYS.WORKBENCH, scope)
    removeStorageItem(STORAGE_KEYS.PENDING_IMAGE_TASKS, scope)
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
    searchEnabled: params.workbenchState.searchEnabled,
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

export function importPlaygroundData(
  file: File
): Promise<PlaygroundImportData> {
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
                .map((session) => normalizeSession(session))
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
