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
import type { PerformanceSeriesPoint } from '@/features/performance-metrics/types'
import { type UptimeDayPoint } from '../lib/mock-stats'
import { UptimeSparkline } from './model-details-uptime-sparkline'

export type ModelPerfBadgeData = {
  avg_latency_ms: number
  success_rate: number
  avg_tps: number
  series?: PerformanceSeriesPoint[]
}

export interface ModelPerfBadgeProps extends React.HTMLAttributes<HTMLDivElement> {
  perf: ModelPerfBadgeData | undefined
}

function formatCompactThroughput(tps: number): string {
  return formatThroughput(tps).replace(' t/s', 'tps')
}

function toUptimeSeries(series: PerformanceSeriesPoint[] = []): UptimeDayPoint[] {
  return series.map((point) => ({
    date: new Date(point.ts * 1000).toISOString(),
    uptime_pct: Math.round(point.success_rate * 100) / 100,
    incidents: point.success_rate < 100 ? 1 : 0,
    outage_minutes: 0,
  }))
}

export const ModelPerfBadge = memo(function ModelPerfBadge(
  props: ModelPerfBadgeProps
) {
  const { t } = useTranslation()

  if (!props.perf) {
    return null
  }

  const { avg_latency_ms, avg_tps, series, success_rate } = props.perf
  const statusSeries = toUptimeSeries(series).slice(-12)

  return (
    <div
      className={cn(
        'hidden w-[150px] grid-cols-[38px_48px_48px] gap-x-2 text-right tabular-nums min-[460px]:grid',
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
        <UptimeSparkline
          size='sm'
          showOverall={false}
          series={statusSeries}
          emptyLabel='—'
          className='justify-end gap-0'
        />
      </div>
    </div>
  )
})
