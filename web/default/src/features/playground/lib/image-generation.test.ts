import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import type { Message, PlaygroundConfig } from '../types'
import {
  buildImageGenerationPayload,
  extractImageGenerationWaitTaskId,
  getImageGenerationWaitMessage,
  imageTaskResultToMarkdown,
  isImageEditRequestPayload,
  parseGeneratedImagesFromMarkdown,
} from './image-generation'
import { formatMessageForAPI } from './message-utils'
import {
  buildChatCompletionPayload,
  buildPlaygroundPreviewPayload,
} from './payload-builder'

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

function assistantErrorMessage(content: string): Message {
  return {
    ...message('assistant', content),
    status: 'error',
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

    assert.match(payload.prompt, /^再来一张/)
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

    assert.match(payload.prompt, /^改成漫画风格/)
    assert.equal(payload.image, 'data:image/png;base64,abc123')
  })

  test('instructs image edits to return a newly generated image instead of the source image', () => {
    const payload = buildImageGenerationPayload(
      [
        message(
          'user',
          '改成漫画风格\n\n![attached image 1](data:image/png;base64,abc123)'
        ),
      ],
      config
    )

    assert.match(payload.prompt, /^改成漫画风格/)
    assert.match(payload.prompt, /return a newly generated edited image/i)
    assert.match(
      payload.prompt,
      /do not return the input\/reference image unchanged/i
    )
    assert.equal(payload.image, 'data:image/png;base64,abc123')
  })

  test('does not reuse previous user attachment images as generated references', () => {
    const payload = buildImageGenerationPayload(
      [
        message(
          'user',
          '改成漫画风格\n\n![original.png](data:image/png;base64,original)'
        ),
        message(
          'assistant',
          '![generated image 1](/pg/images/generations/task_done/image/0)'
        ),
        message('user', '再加一只猫'),
      ],
      config
    )

    assert.match(payload.prompt, /^再加一只猫/)
    assert.equal(payload.image, undefined)
    assert.deepEqual(payload.reference_images, [
      '/pg/images/generations/task_done/image/0',
    ])
  })

  test('marks image inputs as edit requests so Playground uses the edits endpoint', () => {
    const payload = buildImageGenerationPayload(
      [
        message(
          'user',
          '改成漫画风格\n\n![attached image 1](data:image/png;base64,abc123)'
        ),
      ],
      config
    )

    assert.equal(isImageEditRequestPayload(payload), true)
  })

  test('keeps text-only image generation on the generations endpoint', () => {
    const payload = buildImageGenerationPayload(
      [message('user', '生成一张猫咪头像')],
      config
    )

    assert.equal(isImageEditRequestPayload(payload), false)
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

  test('uses the task image endpoint when task result has both url and base64 image data', () => {
    const markdown = imageTaskResultToMarkdown('task_done', {
      task_id: 'task_done',
      status: 'succeeded',
      data: {
        data: [
          {
            url: 'https://chatgpt.com/backend-api/estuary/content?id=source_image',
            b64_json: 'new-image-base64',
          },
        ],
      },
    })

    assert.equal(
      markdown,
      '![generated image 1](/pg/images/generations/task_done/image/0)'
    )
    assert.equal(markdown.includes('source_image'), false)
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

  test('does not render arbitrary user attachments as generated images', () => {
    const parsed = parseGeneratedImagesFromMarkdown(
      'input image\n\n![original.png](data:image/png;base64,original)'
    )

    assert.equal(
      parsed.text,
      'input image\n\n![original.png](data:image/png;base64,original)'
    )
    assert.deepEqual(parsed.images, [])
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
  test('excludes previous playground error assistant messages from API context', () => {
    const payload = buildChatCompletionPayload(
      [
        message('user', '你好'),
        assistantErrorMessage(
          'Request error occurred: Generation was interrupted'
        ),
        message('user', '再试一次'),
      ],
      config,
      {
        temperature: false,
        top_p: false,
        max_tokens: false,
        frequency_penalty: false,
        presence_penalty: false,
        seed: false,
      }
    )

    assert.deepEqual(payload.messages, [
      { role: 'user', content: '你好' },
      { role: 'user', content: '再试一次' },
    ])
  })

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

  test('routes ChatGPT Web image-capable text image requests through image generation preview payload', () => {
    const result = buildPlaygroundPreviewPayload({
      messages: [message('user', '生成一张美女图片')],
      config: { ...config, model: 'gpt-5.5-thinking' },
      parameterEnabled: {
        temperature: false,
        top_p: false,
        max_tokens: false,
        frequency_penalty: false,
        presence_penalty: false,
        seed: false,
      },
      customRequestMode: false,
      customRequestBody: '',
    })

    assert.equal(result.error, null)
    assert.equal(result.payload?.model, 'gpt-5.5-thinking')
    assert.equal(
      (result.payload as { prompt?: string } | null)?.prompt,
      '生成一张美女图片'
    )
  })

  test('routes ChatGPT Web image-capable image edits through image generation preview payload', () => {
    const result = buildPlaygroundPreviewPayload({
      messages: [
        message(
          'user',
          '改成漫画风格\n\n![attached image 1](data:image/png;base64,abc123)'
        ),
      ],
      config: { ...config, model: 'gpt-5.5-thinking' },
      parameterEnabled: {
        temperature: false,
        top_p: false,
        max_tokens: false,
        frequency_penalty: false,
        presence_penalty: false,
        seed: false,
      },
      customRequestMode: false,
      customRequestBody: '',
    })

    assert.equal(result.error, null)
    assert.equal(result.payload?.model, 'gpt-5.5-thinking')
    assert.equal(
      (result.payload as { image?: string } | null)?.image,
      'data:image/png;base64,abc123'
    )
    assert.match(
      String((result.payload as { prompt?: string } | null)?.prompt || ''),
      /return a newly generated edited image/i
    )
  })

  test('routes ChatGPT Web follow-up edits of prior generated images through image generation preview payload', () => {
    const result = buildPlaygroundPreviewPayload({
      messages: [
        message('user', '生成一张猫咪头像'),
        message(
          'assistant',
          '![generated image 1](/pg/images/generations/task_done/image/0)'
        ),
        message('user', '再加一只猫'),
      ],
      config: { ...config, model: 'gpt-5.5-thinking' },
      parameterEnabled: {
        temperature: false,
        top_p: false,
        max_tokens: false,
        frequency_penalty: false,
        presence_penalty: false,
        seed: false,
      },
      customRequestMode: false,
      customRequestBody: '',
    })

    assert.equal(result.error, null)
    assert.equal(result.payload?.model, 'gpt-5.5-thinking')
    assert.deepEqual(
      (result.payload as { reference_images?: string[] } | null)
        ?.reference_images,
      ['/pg/images/generations/task_done/image/0']
    )
    assert.match(
      String((result.payload as { prompt?: string } | null)?.prompt || ''),
      /return a newly generated edited image/i
    )
  })

  test('keeps ChatGPT Web image understanding prompts on chat completions', () => {
    const result = buildPlaygroundPreviewPayload({
      messages: [
        message(
          'user',
          '请描述这张图\n\n![attached image 1](data:image/png;base64,abc123)'
        ),
      ],
      config: { ...config, model: 'gpt-5.5-thinking' },
      parameterEnabled: {
        temperature: false,
        top_p: false,
        max_tokens: false,
        frequency_penalty: false,
        presence_penalty: false,
        seed: false,
      },
      customRequestMode: false,
      customRequestBody: '',
    })

    const payload = result.payload as { messages?: unknown[]; prompt?: string }
    assert.equal(result.error, null)
    assert.ok(Array.isArray(payload.messages))
    assert.equal(payload.prompt, undefined)
  })

  test('keeps ChatGPT Web image style questions on chat completions', () => {
    const result = buildPlaygroundPreviewPayload({
      messages: [
        message(
          'user',
          '这张图是什么风格？\n\n![attached image 1](data:image/png;base64,abc123)'
        ),
      ],
      config: { ...config, model: 'gpt-5.5-thinking' },
      parameterEnabled: {
        temperature: false,
        top_p: false,
        max_tokens: false,
        frequency_penalty: false,
        presence_penalty: false,
        seed: false,
      },
      customRequestMode: false,
      customRequestBody: '',
    })

    const payload = result.payload as { messages?: unknown[]; prompt?: string }
    assert.equal(result.error, null)
    assert.ok(Array.isArray(payload.messages))
    assert.equal(payload.prompt, undefined)
  })
})
