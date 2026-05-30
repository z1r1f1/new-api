import assert from 'node:assert/strict'
import { beforeEach, describe, test } from 'node:test'
import type {
  Message,
  PendingImageGenerationTask,
  PlaygroundSession,
} from '../types'
import {
  clearActivePlaygroundChatMessage,
  clearPlaygroundData,
  loadConfig,
  loadMessages,
  loadPendingImageTasks,
  loadSessionState,
  markActivePlaygroundChatMessage,
  saveActiveSessionId,
  saveConfig,
  saveMessages,
  saveSessions,
  updateStoredSessionMessages,
  upsertPendingImageTask,
} from './storage'

class MemoryStorage implements Storage {
  private readonly store = new Map<string, string>()

  get length(): number {
    return this.store.size
  }

  clear(): void {
    this.store.clear()
  }

  getItem(key: string): string | null {
    return this.store.get(key) ?? null
  }

  key(index: number): string | null {
    return Array.from(this.store.keys())[index] ?? null
  }

  removeItem(key: string): void {
    this.store.delete(key)
  }

  setItem(key: string, value: string): void {
    this.store.set(key, value)
  }
}

Object.defineProperty(globalThis, 'localStorage', {
  value: new MemoryStorage(),
  configurable: true,
})

function session(
  id: string,
  title: string,
  messages: Message[] = []
): PlaygroundSession {
  const now = '2026-05-25T00:00:00.000Z'
  return {
    id,
    title,
    messages,
    createdAt: now,
    updatedAt: now,
  }
}

function message(key: string, content: string): Message {
  return {
    key,
    from: 'user',
    versions: [{ id: `${key}-v1`, content }],
  }
}

function assistantMessage(
  key: string,
  content: string,
  status: Message['status']
): Message {
  return {
    key,
    from: 'assistant',
    versions: [{ id: `${key}-v1`, content }],
    status,
  }
}

function pendingTask(
  taskId: string,
  sessionId: string
): PendingImageGenerationTask {
  return {
    taskId,
    sessionId,
    messageKey: `${taskId}-message`,
    debugId: `${taskId}-debug`,
    startedAt: 1_777_000_000_000,
    updatedAt: '2026-05-25T00:00:00.000Z',
  }
}

beforeEach(() => {
  localStorage.clear()
  clearActivePlaygroundChatMessage('active-chat-message', 1)
  clearActivePlaygroundChatMessage('stale-chat-message', 1)
})

describe('playground storage user scope', () => {
  test('isolates sessions and active session by user id', () => {
    saveSessions([session('user-1-session', 'User 1')], 1)
    saveActiveSessionId('user-1-session', 1)

    saveSessions([session('user-2-session', 'User 2')], 2)
    saveActiveSessionId('user-2-session', 2)

    const user1State = loadSessionState(1)
    const user2State = loadSessionState(2)

    assert.equal(user1State.activeSessionId, 'user-1-session')
    assert.equal(user1State.sessions[0].title, 'User 1')
    assert.equal(user2State.activeSessionId, 'user-2-session')
    assert.equal(user2State.sessions[0].title, 'User 2')
  })

  test('isolates messages, config, and pending image tasks by user id', () => {
    saveMessages([message('u1-message', 'hello from user 1')], 1)
    saveConfig({ model: 'gpt-4o', group: 'vip' }, 1)
    upsertPendingImageTask(pendingTask('task-user-1', 'session-1'), 1)

    saveMessages([message('u2-message', 'hello from user 2')], 2)
    saveConfig({ model: 'gpt-image-2', group: 'svip' }, 2)
    upsertPendingImageTask(pendingTask('task-user-2', 'session-2'), 2)

    assert.equal(loadMessages(1)?.[0]?.key, 'u1-message')
    assert.equal(loadMessages(2)?.[0]?.key, 'u2-message')
    assert.equal(loadConfig(1).model, 'gpt-4o')
    assert.equal(loadConfig(2).model, 'gpt-image-2')
    assert.deepEqual(
      loadPendingImageTasks(1).map((task) => task.taskId),
      ['task-user-1']
    )
    assert.deepEqual(
      loadPendingImageTasks(2).map((task) => task.taskId),
      ['task-user-2']
    )
  })

  test('updates inactive session messages in durable storage', () => {
    saveSessions(
      [
        session('active-session', 'Active', [
          message('active-message', 'active'),
        ]),
        session('background-session', 'Background', [
          message('background-message', 'old'),
        ]),
      ],
      1
    )
    saveActiveSessionId('active-session', 1)
    saveMessages([message('active-message', 'active')], 1)

    const result = updateStoredSessionMessages(
      'background-session',
      (messages) =>
        messages.map((item) =>
          item.key === 'background-message'
            ? {
                ...item,
                versions: [{ ...item.versions[0], content: 'new' }],
              }
            : item
        ),
      1
    )

    const reloaded = loadSessionState(1)
    const backgroundSession = reloaded.sessions.find(
      (item) => item.id === 'background-session'
    )

    assert.equal(result.updated, true)
    assert.equal(result.activeSessionId, 'active-session')
    assert.equal(backgroundSession?.messages[0]?.versions[0]?.content, 'new')
    assert.equal(loadMessages(1)?.[0]?.versions[0]?.content, 'active')
  })

  test('preserves active streaming chat messages during durable storage reloads', () => {
    saveSessions(
      [
        session('active-session', 'Active', [
          assistantMessage('active-chat-message', '', 'loading'),
        ]),
      ],
      1
    )
    saveActiveSessionId('active-session', 1)

    markActivePlaygroundChatMessage('active-chat-message', 1)

    const reloaded = loadSessionState(1)
    assert.equal(reloaded.sessions[0].messages[0].status, 'loading')
    assert.equal(reloaded.sessions[0].messages[0].versions[0].content, '')

    const result = updateStoredSessionMessages(
      'active-session',
      (messages) =>
        messages.map((item) =>
          item.key === 'active-chat-message'
            ? {
                ...item,
                versions: [{ ...item.versions[0], content: 'streamed text' }],
                status: 'streaming',
              }
            : item
        ),
      1
    )

    assert.equal(result.messages?.[0]?.status, 'streaming')
    assert.equal(result.messages?.[0]?.versions[0]?.content, 'streamed text')
  })

  test('sanitizes stale streaming chat messages without an active stream marker', () => {
    saveSessions(
      [
        session('active-session', 'Active', [
          assistantMessage('stale-chat-message', '', 'loading'),
        ]),
      ],
      1
    )
    saveActiveSessionId('active-session', 1)

    const reloaded = loadSessionState(1)

    assert.equal(reloaded.sessions[0].messages[0].status, 'error')
    assert.match(
      reloaded.sessions[0].messages[0].versions[0].content,
      /Generation was interrupted/
    )
  })

  test('clears only the selected user scope', () => {
    saveSessions([session('user-1-session', 'User 1')], 1)
    saveSessions([session('user-2-session', 'User 2')], 2)
    saveMessages([message('u1-message', 'hello from user 1')], 1)
    saveMessages([message('u2-message', 'hello from user 2')], 2)

    clearPlaygroundData(1)

    assert.notEqual(loadSessionState(1).activeSessionId, 'user-1-session')
    assert.equal(loadSessionState(2).activeSessionId, 'user-2-session')
    assert.equal(loadMessages(1), null)
    assert.equal(loadMessages(2)?.[0]?.key, 'u2-message')
  })
})
