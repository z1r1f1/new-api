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

For commercial licensing, please contact support@quantumnous.com
*/
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import type { LogOtherData } from '../types'
import { getUsageLogRequestMetadata } from './request-metadata'

describe('usage log request metadata', () => {
  test('restores fast, service tier and reasoning detail metadata from log other', () => {
    const metadata = getUsageLogRequestMetadata({
      request_fast: false,
      request_fast_service_tier: 'priority',
      request_service_tier: 'priority',
      response_service_tier: 'default',
      request_effort: 'high',
      reasoning_effort: 'medium',
      request_protocol: 'WEBSOCKET',
    } as LogOtherData)

    assert.equal(metadata.requestProtocol, 'websocket')
    assert.equal(metadata.showRequestFast, true)
    assert.equal(metadata.requestFast, true)
    assert.equal(metadata.requestFastServiceTier, 'priority')
    assert.equal(metadata.requestServiceTier, 'priority')
    assert.equal(metadata.responseServiceTier, 'default')
    assert.equal(metadata.requestEffort, 'high')
    assert.equal(metadata.reasoningEffort, 'medium')
    assert.equal(metadata.showFinalReasoningEffort, true)
  })

  test('does not duplicate final reasoning effort when it matches request effort', () => {
    const metadata = getUsageLogRequestMetadata({
      request_effort: 'Medium',
      reasoning_effort: 'medium',
    } as LogOtherData)

    assert.equal(metadata.requestEffort, 'Medium')
    assert.equal(metadata.reasoningEffort, 'medium')
    assert.equal(metadata.showFinalReasoningEffort, false)
  })
})
