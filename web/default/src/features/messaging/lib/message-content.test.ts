import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  buildChatImageMarkdown,
  extractMessageContentParts,
  isChatMessageSendable,
  isSafeChatImageDataUrl,
} from './message-content'

describe('message content image parsing', () => {
  test('keeps plain text as a text part', () => {
    assert.deepEqual(extractMessageContentParts('hello\nworld'), [
      { type: 'text', text: 'hello\nworld' },
    ])
  })

  test('extracts safe data-url image markdown while preserving surrounding text', () => {
    assert.deepEqual(
      extractMessageContentParts(
        'before\n![pasted-image.png](data:image/png;base64,QUJDRA==)\nafter'
      ),
      [
        { type: 'text', text: 'before\n' },
        {
          type: 'image',
          alt: 'pasted-image.png',
          src: 'data:image/png;base64,QUJDRA==',
        },
        { type: 'text', text: '\nafter' },
      ]
    )
  })

  test('does not render unsafe or remote image markdown as an image part', () => {
    const unsafe =
      '![bad](javascript:alert(1)) and ![remote](https://example.com/a.png)'
    assert.deepEqual(extractMessageContentParts(unsafe), [
      { type: 'text', text: unsafe },
    ])
  })

  test('validates chat image data-url mime types and base64 payloads', () => {
    assert.equal(isSafeChatImageDataUrl('data:image/webp;base64,AAAA'), true)
    assert.equal(isSafeChatImageDataUrl('data:image/jpeg;base64,A+/='), true)
    assert.equal(
      isSafeChatImageDataUrl('data:image/svg+xml;base64,AAAA'),
      false
    )
    assert.equal(isSafeChatImageDataUrl('data:image/png;base64,<svg>'), false)
  })

  test('allows sending an image-only message', () => {
    assert.equal(
      isChatMessageSendable('', [
        {
          id: '1',
          name: 'pasted.png',
          mimeType: 'image/png',
          size: 4,
          dataUrl: 'data:image/png;base64,AAAA',
        },
      ]),
      true
    )
    assert.equal(isChatMessageSendable('   ', []), false)
  })

  test('escapes attachment names when building image markdown', () => {
    assert.equal(
      buildChatImageMarkdown({
        id: '1',
        name: 'screen [1](copy).png',
        mimeType: 'image/png',
        size: 4,
        dataUrl: 'data:image/png;base64,AAAA',
      }),
      '![screen 1 copy.png](data:image/png;base64,AAAA)'
    )
  })
})
