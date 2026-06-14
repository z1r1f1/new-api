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
import type { LogOtherData } from '../types'

export interface UsageLogRequestMetadata {
  requestProtocol: string
  requestServiceTier: string
  requestFastServiceTier: string
  responseServiceTier: string
  requestEffort: string
  reasoningEffort: string
  showRequestFast: boolean
  requestFast: boolean
  showFinalReasoningEffort: boolean
}

function normalizeString(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

function isFastServiceTierValue(value: string): boolean {
  const normalized = value.trim().toLowerCase()
  return normalized === 'fast' || normalized === 'priority'
}

export function getUsageLogRequestMetadata(
  other: LogOtherData | null | undefined
): UsageLogRequestMetadata {
  const requestProtocol = normalizeString(other?.request_protocol).toLowerCase()
  const requestServiceTier = normalizeString(other?.request_service_tier)
  const requestFastServiceTier = normalizeString(
    other?.request_fast_service_tier
  )
  const responseServiceTier = normalizeString(other?.response_service_tier)
  const requestEffort = normalizeString(other?.request_effort)
  const reasoningEffort = normalizeString(other?.reasoning_effort)
  const showRequestFast =
    typeof other?.request_fast === 'boolean' ||
    Boolean(
      other?.request_path ||
      requestServiceTier ||
      responseServiceTier ||
      requestEffort ||
      reasoningEffort
    )
  const requestFast =
    other?.request_fast === true ||
    isFastServiceTierValue(requestServiceTier) ||
    isFastServiceTierValue(requestFastServiceTier)
  const showFinalReasoningEffort =
    Boolean(reasoningEffort) &&
    reasoningEffort.toLowerCase() !== requestEffort.toLowerCase()

  return {
    requestProtocol,
    requestServiceTier,
    requestFastServiceTier,
    responseServiceTier,
    requestEffort,
    reasoningEffort,
    showRequestFast,
    requestFast,
    showFinalReasoningEffort,
  }
}
