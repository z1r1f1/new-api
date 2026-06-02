import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  LOG_TYPE_ENUM,
  LOG_TYPE_FILTERS,
  LOG_TYPE_ALL_VALUE,
} from './constants'

describe('usage log type filters', () => {
  test('labels error logs as error requests while preserving type=5 filtering', () => {
    const errorFilter = LOG_TYPE_FILTERS.find(
      (filter) => filter.value === String(LOG_TYPE_ENUM.ERROR)
    )

    assert.deepEqual(errorFilter, {
      label: 'Error Requests',
      value: '5',
    })
  })

  test('keeps all-types sentinel but hides display-only unknown from filters', () => {
    assert.deepEqual(LOG_TYPE_FILTERS[0], {
      label: 'All Types',
      value: LOG_TYPE_ALL_VALUE,
    })
    assert.equal(
      LOG_TYPE_FILTERS.some((filter) => filter.label === 'Unknown'),
      false
    )
  })
})
