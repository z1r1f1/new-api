/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
*/
import { expect, it } from 'vitest'

import { buildSearchParams } from '../filter'
import { getCacheHitRate } from '../format'
import { buildApiParams } from '../utils'

it('serializes the common channel ID filter with the backend parameter name', () => {
  const searchParams = buildSearchParams({ channel: '123' }, 'common')

  expect(searchParams).toEqual({ channelId: '123' })
  expect(searchParams).not.toHaveProperty('channel')
})

it('passes a channel ID URL filter to the common logs API', () => {
  const params = buildApiParams({
    page: 1,
    pageSize: 20,
    searchParams: { channelId: '123' },
    isAdmin: true,
  })

  expect(params).toMatchObject({ channel_id: 123 })
  expect(params).not.toHaveProperty('channel')
})

it('omits an empty channel ID filter', () => {
  const params = buildApiParams({
    page: 1,
    pageSize: 20,
    searchParams: { channelId: '' },
    isAdmin: true,
  })

  expect(params).not.toHaveProperty('channel_id')
})

it('calculates a row cache hit rate from normalized input tokens', () => {
  expect(
    getCacheHitRate(
      { prompt_tokens: 100 },
      { cache_tokens: 30, input_tokens_total: 120 }
    )
  ).toBe(25)
})

it('does not produce a cache hit rate without input tokens', () => {
  expect(getCacheHitRate({ prompt_tokens: 0 }, {})).toBeNull()
})

it('uses the full Anthropic input total for cache hit rate', () => {
  expect(
    getCacheHitRate(
      { prompt_tokens: 2 },
      {
        usage_semantic: 'anthropic',
        cache_tokens: 198875,
        cache_write_tokens: 1095,
        admin_info: {
          usage_billing_path: 'billing-usage-anthropic',
        },
      }
    )
  ).toBeCloseTo((198875 / 199972) * 100)
})

it('falls back to split Anthropic cache creation tokens', () => {
  expect(
    getCacheHitRate(
      { prompt_tokens: 2 },
      {
        usage_semantic: 'anthropic',
        cache_tokens: 100,
        cache_creation_tokens_5m: 20,
        cache_creation_tokens_1h: 30,
        admin_info: {
          usage_billing_path: 'billing-usage-anthropic-estimated',
        },
      }
    )
  ).toBeCloseTo((100 / 152) * 100)
})
