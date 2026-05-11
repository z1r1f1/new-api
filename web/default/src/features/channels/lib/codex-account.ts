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
import type { StatusBadgeProps } from '@/components/status-badge'

export type CodexAccountBadge = {
  label: string
  variant: StatusBadgeProps['variant']
}

export function normalizeCodexPlanType(value: unknown): string {
  if (value == null) return ''
  return String(value).trim().toLowerCase()
}

const CODEX_PLAN_TYPE_BADGE: Record<string, CodexAccountBadge> = {
  enterprise: { label: 'Enterprise', variant: 'success' },
  team: { label: 'Team', variant: 'info' },
  pro: { label: 'Pro', variant: 'blue' },
  plus: { label: 'Plus', variant: 'purple' },
  free: { label: 'Free', variant: 'warning' },
}

export function getCodexAccountTypeBadge(
  value: unknown,
  t: (key: string) => string
): CodexAccountBadge {
  const normalized = normalizeCodexPlanType(value)
  return (
    CODEX_PLAN_TYPE_BADGE[normalized] ?? {
      label: String(value || '') || t('Unknown'),
      variant: 'neutral',
    }
  )
}
