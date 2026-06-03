import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  detectUnixTimestampUnit,
  formatChannelAffinityKeySource,
  formatLogTimestampToDate,
  getVisibleAdminRequestHeaders,
  shouldShowRequestConversion,
} from './format'

describe('usage log timestamp formatting', () => {
  test('formats millisecond submit timestamps as current dates instead of far-future years', () => {
    const formatted = formatLogTimestampToDate(1779873671802)

    assert.match(formatted, /^2026-05-27 /)
    assert.doesNotMatch(formatted, /^583/)
  })

  test('detects numeric string millisecond timestamps from API payloads', () => {
    assert.equal(detectUnixTimestampUnit('1779873671802'), 'milliseconds')
    assert.match(formatLogTimestampToDate('1779873671802'), /^2026-05-27 /)
  })
})

describe('usage log request conversion visibility', () => {
  test('shows request conversion metadata for ordinary consume logs', () => {
    assert.equal(
      shouldShowRequestConversion(
        {
          request_path: '/v1/messages',
          request_conversion: ['Claude Messages', 'OpenAI Responses'],
        },
        2
      ),
      true
    )
  })

  test('hides request conversion metadata for refund logs', () => {
    assert.equal(
      shouldShowRequestConversion(
        {
          request_path: '/v1/messages',
          request_conversion: ['Claude Messages', 'OpenAI Responses'],
        },
        6
      ),
      false
    )
  })
})

describe('channel affinity key source formatting', () => {
  test('combines matched key source type and path', () => {
    assert.equal(
      formatChannelAffinityKeySource({
        key_source: 'gjson',
        key_path: 'prompt_cache_key',
      }),
      'gjson:prompt_cache_key'
    )
  })

  test('uses context key when the fallback source is a context id', () => {
    assert.equal(
      formatChannelAffinityKeySource({
        key_source: 'context_int',
        key_key: 'token_id',
      }),
      'context_int:token_id'
    )
  })

  test('returns an empty string when no source metadata exists', () => {
    assert.equal(formatChannelAffinityKeySource({}), '')
    assert.equal(formatChannelAffinityKeySource(null), '')
  })
})

describe('admin request header visibility formatting', () => {
  test('returns sorted request headers for admins only', () => {
    assert.deepEqual(
      getVisibleAdminRequestHeaders(
        {
          request_headers: {
            'X-Request-Id': 'req-1',
            'Content-Type': 'application/json',
          },
        },
        true
      ),
      [
        { key: 'Content-Type', value: 'application/json' },
        { key: 'X-Request-Id', value: 'req-1' },
      ]
    )

    assert.deepEqual(
      getVisibleAdminRequestHeaders(
        {
          request_headers: {
            'X-Request-Id': 'req-1',
          },
        },
        false
      ),
      []
    )
  })

  test('filters blank request header keys and values', () => {
    assert.deepEqual(
      getVisibleAdminRequestHeaders(
        {
          request_headers: {
            '': 'empty-key',
            'X-Blank': '   ',
            'X-Ok': 'ok',
          },
        },
        true
      ),
      [{ key: 'X-Ok', value: 'ok' }]
    )
  })
})
