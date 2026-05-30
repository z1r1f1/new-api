import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  buildImageGenerationPayload,
  extractImageGenerationWaitTaskId,
  getImageGenerationWaitMessage,
  imageTaskResultToMarkdown,
} from './image-generation'
import { formatMessageForAPI } from './message-utils'
import { buildChatCompletionPayload } from './payload-builder'
import type { Message, PlaygroundConfig } from '../types'

const config: PlaygroundConfig = {
  model: 'gpt-image-2',
  group: 'vip',
  temperature: 0.7,
  top_p: 1,
  max_tokens: 4096,
  frequency_penalty: 0,
  presence_penalty: 0,
  seed: null,
  stream: true,
  deep_research: false,
}

function message(from: Message['from'], content: string): Message {
  return {
    key: `${from}-${content}`,
    from,
    versions: [{ id: 'v1', content }],
    status: from === 'assistant' ? 'complete' : undefined,
  }
}

describe('buildImageGenerationPayload', () => {
  test('uses the latest generated playground image as reference for follow-up image generation', () => {
    const payload = buildImageGenerationPayload(
      [
        message('user', '生成一张美女图片'),
        message(
          'assistant',
          '![generated image 1](/pg/images/generations/task_first/image/0)'
        ),
        message('user', '再来一张'),
      ],
      config
    )

    assert.equal(payload.prompt, '再来一张')
    assert.deepEqual(payload.reference_images, [
      '/pg/images/generations/task_first/image/0',
    ])
  })

  test('sends images attached to the latest user message as the edit image input', () => {
    const payload = buildImageGenerationPayload(
      [
        message(
          'user',
          '改成漫画风格\n\n![attached image 1](data:image/png;base64,abc123)'
        ),
      ],
      config
    )

    assert.equal(payload.prompt, '改成漫画风格')
    assert.equal(payload.image, 'data:image/png;base64,abc123')
  })
})

describe('image generation task rendering', () => {
  test('uses the playground task image endpoint for completed base64 task results', () => {
    const markdown = imageTaskResultToMarkdown('task_done', {
      task_id: 'task_done',
      status: 'succeeded',
      data: {
        data: [{ b64_json: 'abc123' }],
      },
    })

    assert.equal(
      markdown,
      '![generated image 1](/pg/images/generations/task_done/image/0)'
    )
  })

  test('computes wait elapsed from the original task start time', () => {
    const originalNow = Date.now
    Date.now = () => 1_777_000_065_000
    try {
      const message = getImageGenerationWaitMessage(
        'task_waiting',
        null,
        0,
        1_777_000_000_000,
        (key, options) =>
          key === 'Elapsed: {{elapsed}}'
            ? `Elapsed: ${String(options?.elapsed || '')}`
            : key
      )

      assert.match(message, /Elapsed: 1m 5s/)
      assert.match(message, /task_waiting/)
    } finally {
      Date.now = originalNow
    }
  })

  test('extracts task id from localized wait messages for legacy recovery', () => {
    assert.equal(
      extractImageGenerationWaitTaskId(
        '正在生成图片.\n\n已等待：55s\n\n任务 ID： `task_q6Fx1L8auCV58BZC5imnR6yBu4ZXNRgD`'
      ),
      'task_q6Fx1L8auCV58BZC5imnR6yBu4ZXNRgD'
    )
    assert.equal(
      extractImageGenerationWaitTaskId('Task ID: `task_abc123`'),
      'task_abc123'
    )
    assert.equal(extractImageGenerationWaitTaskId('Task ID: `resp_abc`'), '')
  })
})

describe('formatMessageForAPI', () => {
  test('converts markdown image attachments into chat image_url content parts', () => {
    const formatted = formatMessageForAPI(
      message(
        'user',
        '请描述这张图\n\n![attached image 1](data:image/png;base64,abc123)'
      )
    )

    assert.deepEqual(formatted.content, [
      { type: 'text', text: '请描述这张图' },
      {
        type: 'image_url',
        image_url: { url: 'data:image/png;base64,abc123' },
      },
    ])
  })
})

describe('buildChatCompletionPayload', () => {
  test('adds web search options when search is enabled', () => {
    const payload = buildChatCompletionPayload(
      [message('user', 'hi')],
      config,
      {
        temperature: false,
        top_p: false,
        max_tokens: false,
        frequency_penalty: false,
        presence_penalty: false,
        seed: false,
      },
      { searchEnabled: true }
    )

    assert.deepEqual(payload.web_search_options, {
      search_context_size: 'medium',
    })
  })

  test('adds ChatGPT Web deep research flag when deep research is enabled', () => {
    const payload = buildChatCompletionPayload(
      [message('user', 'hi')],
      { ...config, model: 'gpt-5.5-thinking', deep_research: true },
      {
        temperature: false,
        top_p: false,
        max_tokens: false,
        frequency_penalty: false,
        presence_penalty: false,
        seed: false,
      }
    )

    assert.equal(payload.chatgpt_web_deep_research, true)
  })
})
