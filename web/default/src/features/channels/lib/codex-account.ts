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
