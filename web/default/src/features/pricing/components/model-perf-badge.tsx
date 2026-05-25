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
import { memo } from 'react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import {
  formatLatency,
  formatThroughput,
} from '@/features/performance-metrics/lib/format'

export type ModelPerfBadgeData = {
  avg_latency_ms: number
  success_rate: number
  avg_tps: number
}

export interface ModelPerfBadgeProps extends React.HTMLAttributes<HTMLDivElement> {
  perf: ModelPerfBadgeData | undefined
}

function formatCompactThroughput(tps: number): string {
  return formatThroughput(tps).replace(' t/s', 'tps')
}

const STATUS_SEGMENTS = 5

function getStatusColour(successRate: number): string {
  if (successRate >= 99.9) return 'bg-emerald-500'
  if (successRate >= 99) return 'bg-emerald-400'
  if (successRate >= 95) return 'bg-amber-500'
  if (successRate >= 90) return 'bg-amber-600'
  return 'bg-rose-500'
}

function getActiveStatusSegments(successRate: number): number {
  if (successRate >= 99.9) return 5
  if (successRate >= 99) return 4
  if (successRate >= 95) return 3
  if (successRate >= 90) return 2
  return 1
}

export const ModelPerfBadge = memo(function ModelPerfBadge(
  props: ModelPerfBadgeProps
) {
  const { t } = useTranslation()

  if (!props.perf) {
    return null
  }

  const { avg_latency_ms, avg_tps, success_rate } = props.perf
  const statusColour = getStatusColour(success_rate)
  const activeStatusSegments = getActiveStatusSegments(success_rate)

  return (
    <div
      className={cn(
        'hidden w-[142px] grid-cols-[38px_48px_40px] gap-x-2 text-right tabular-nums min-[460px]:grid',
        props.className
      )}
    >
      <div title={t('Average latency')} className='min-w-0'>
        <div className='text-muted-foreground/55 text-[10px] leading-4'>
          {t('Latency short')}
        </div>
        <div className='text-muted-foreground/80 font-mono text-xs leading-4 whitespace-nowrap'>
          {avg_latency_ms > 0 ? formatLatency(avg_latency_ms) : '—'}
        </div>
      </div>
      <div title={t('Throughput')} className='min-w-0'>
        <div className='text-muted-foreground/55 truncate text-[10px] leading-4'>
          {t('Throughput short')}
        </div>
        <div className='text-muted-foreground/80 font-mono text-xs leading-4 whitespace-nowrap'>
          {formatCompactThroughput(avg_tps)}
        </div>
      </div>
      <div
        title={`${t('Success rate')}: ${success_rate.toFixed(1)}%`}
        className='min-w-0'
      >
        <div className='text-muted-foreground/55 truncate text-[10px] leading-4'>
          {t('Status short')}
        </div>
        <div className='flex h-4 items-end justify-end gap-0.5'>
          {Array.from({ length: STATUS_SEGMENTS }, (_, index) => {
            const isActive = index < activeStatusSegments
            return (
              <span
                key={index}
                className={cn(
                  'h-3 w-1 rounded-full',
                  isActive ? statusColour : 'bg-muted-foreground/15'
                )}
              />
            )
          })}
        </div>
      </div>
    </div>
  )
})
