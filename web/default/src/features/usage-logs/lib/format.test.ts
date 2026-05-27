import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  detectUnixTimestampUnit,
  formatLogTimestampToDate,
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
