import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  AVAILABLE_CHAT_REACTIONS,
  AVAILABLE_CHAT_STICKERS,
  buildChatImageMarkdown,
  buildChatStickerMessage,
  extractMessageContentParts,
  extractMessageReplyReference,
  getMessageReplyPreview,
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

  test('allows sending a sticker-only message', () => {
    assert.equal(
      isChatMessageSendable('', [], [AVAILABLE_CHAT_STICKERS[0]]),
      true
    )
    assert.equal(isChatMessageSendable('   ', [], []), false)
  })

  test('builds and extracts a reply reference without leaking it into content parts', () => {
    const reply = {
      messageId: 42,
      senderName: 'Alice',
      preview: 'hello from earlier',
    }
    const body = buildChatStickerMessage(AVAILABLE_CHAT_STICKERS[1], reply)

    assert.deepEqual(extractMessageReplyReference(body), reply)
    assert.deepEqual(extractMessageContentParts(body), [
      { type: 'sticker', sticker: AVAILABLE_CHAT_STICKERS[1] },
    ])
  })

  test('builds and extracts sticker messages', () => {
    const sticker = AVAILABLE_CHAT_STICKERS[2]
    const body = buildChatStickerMessage(sticker)

    assert.deepEqual(extractMessageContentParts(body), [
      { type: 'sticker', sticker },
    ])
    assert.equal(
      getMessageReplyPreview(body),
      sticker.emoji
    )
  })

  test('offers a richer plain emoji reaction set', () => {
    assert.ok(AVAILABLE_CHAT_REACTIONS.length >= 40)
    assert.ok(AVAILABLE_CHAT_REACTIONS.every((reaction) => reaction.emoji))
    assert.ok(AVAILABLE_CHAT_REACTIONS.some((reaction) => reaction.emoji === '👍'))
    assert.ok(AVAILABLE_CHAT_REACTIONS.some((reaction) => reaction.emoji === '❤️'))
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
