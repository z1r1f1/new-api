import assert from 'node:assert/strict'
import { beforeEach, describe, test } from 'node:test'
import type {
  Message,
  PendingImageGenerationTask,
  PlaygroundSession,
} from '../types'
import {
  clearPlaygroundData,
  loadConfig,
  loadMessages,
  loadPendingImageTasks,
  loadSessionState,
  saveActiveSessionId,
  saveConfig,
  saveMessages,
  saveSessions,
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
