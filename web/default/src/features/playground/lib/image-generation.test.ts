import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { buildImageGenerationPayload } from './image-generation'
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
})
