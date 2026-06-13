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
import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import {
  type ColumnDef,
  type OnChangeFn,
  type VisibilityState,
} from '@tanstack/react-table'
import { useMediaQuery } from '@/hooks'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { cn } from '@/lib/utils'
import { useIsAdmin } from '@/hooks/use-admin'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import {
  DataTablePage,
  DataTableRow,
  useDataTable,
} from '@/components/data-table'
import {
  DEFAULT_LOGS_DATA,
  LOG_TYPE_ALL_VALUE,
  LOG_TYPE_ENUM,
} from '../constants'
import { useColumnsByCategory } from '../lib/columns'
import { fetchLogsByCategory } from '../lib/utils'
import type { LogCategory } from '../types'
import { CommonLogsFilterBar } from './common-logs-filter-bar'
import { TaskLogsFilterBar } from './task-logs-filter-bar'
import { UsageLogsMobileList } from './usage-logs-mobile-card'

const route = getRouteApi('/_authenticated/usage-logs/$section')

const logTypeRowTint: Record<number, string> = {
  [LOG_TYPE_ENUM.ERROR]: 'bg-rose-50/40 dark:bg-rose-950/20',
  [LOG_TYPE_ENUM.REFUND]: 'bg-blue-50/30 dark:bg-blue-950/15',
}

function deserializeLogTypeFilter(value: unknown): unknown[] {
  const values = Array.isArray(value) ? value : value ? [value] : []
  return values.filter((item) => String(item) !== LOG_TYPE_ALL_VALUE)
}

const USAGE_LOGS_COLUMN_VISIBILITY_STORAGE_KEY_PREFIX =
  'usage-logs-column-visibility'

type StoredColumnVisibilityState = {
  logCategory: LogCategory
  visibility: VisibilityState
}

function getDefaultColumnVisibility(logCategory: LogCategory): VisibilityState {
  if (logCategory === 'common') {
    return {
      response_service_tier: false,
    }
  }

  return {}
}

function getColumnVisibilityStorageKey(logCategory: LogCategory): string {
  return `${USAGE_LOGS_COLUMN_VISIBILITY_STORAGE_KEY_PREFIX}:${logCategory}`
}

function isVisibilityState(value: unknown): value is VisibilityState {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) {
    return false
  }

  return Object.values(value).every((item) => typeof item === 'boolean')
}

function readColumnVisibility(logCategory: LogCategory): VisibilityState {
  const defaultVisibility = getDefaultColumnVisibility(logCategory)

  if (typeof window === 'undefined') {
    return defaultVisibility
  }

  const saved = window.localStorage.getItem(
    getColumnVisibilityStorageKey(logCategory)
  )

  if (!saved) {
    return defaultVisibility
  }

  try {
    const parsed: unknown = JSON.parse(saved)
    if (!isVisibilityState(parsed)) {
      return defaultVisibility
    }

    return {
      ...defaultVisibility,
      ...parsed,
    }
  } catch {
    return defaultVisibility
  }
}

function writeColumnVisibility(
  logCategory: LogCategory,
  visibility: VisibilityState
): void {
  if (typeof window === 'undefined') {
    return
  }

  window.localStorage.setItem(
    getColumnVisibilityStorageKey(logCategory),
    JSON.stringify(visibility)
  )
}

interface UsageLogsTableProps {
  logCategory: LogCategory
}

export function UsageLogsTable({ logCategory }: UsageLogsTableProps) {
  const { t } = useTranslation()
  const isAdmin = useIsAdmin()
  const isMobile = useMediaQuery('(max-width: 640px)')
  const searchParams = route.useSearch()

  const {
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange,
    ensurePageInRange,
  } = useTableUrlState({
    search: route.useSearch(),
    navigate: route.useNavigate(),
    pagination: { defaultPage: 1, defaultPageSize: isMobile ? 20 : 100 },
    globalFilter: { enabled: false },
    columnFilters: [
      {
        columnId: 'created_at',
        searchKey: 'type',
        type: 'array' as const,
        deserialize: deserializeLogTypeFilter,
      },
      { columnId: 'model_name', searchKey: 'model', type: 'string' as const },
      { columnId: 'token_name', searchKey: 'token', type: 'string' as const },
      { columnId: 'group', searchKey: 'group', type: 'string' as const },
      ...(isAdmin
        ? [
            {
              columnId: 'channel',
              searchKey: 'channel',
              type: 'string' as const,
            },
            {
              columnId: 'username',
              searchKey: 'username',
              type: 'string' as const,
            },
          ]
        : []),
    ],
  })

  const { data, isLoading, isFetching } = useQuery({
    queryKey: [
      'logs',
      logCategory,
      isAdmin,
      pagination.pageIndex + 1,
      pagination.pageSize,
      columnFilters,
      searchParams,
      t,
    ],
    queryFn: async () => {
      const result = await fetchLogsByCategory({
        logCategory,
        isAdmin,
        page: pagination.pageIndex + 1,
        pageSize: pagination.pageSize,
        searchParams,
        columnFilters,
      })

      if (!result?.success) {
        toast.error(result?.message || t('Failed to load logs'))
        return DEFAULT_LOGS_DATA
      }

      return result.data || DEFAULT_LOGS_DATA
    },
    placeholderData: (previousData, previousQuery) => {
      if (previousQuery?.queryKey[1] === logCategory) {
        return previousData
      }
      return undefined
    },
  })

  const logs = data?.items || []
  const columns = useColumnsByCategory(logCategory, isAdmin)
  const isLoadingData = isLoading || (isFetching && !data)

  const [columnVisibilityState, setColumnVisibilityState] =
    useState<StoredColumnVisibilityState>(() => ({
      logCategory,
      visibility: readColumnVisibility(logCategory),
    }))

  const columnVisibility =
    columnVisibilityState.logCategory === logCategory
      ? columnVisibilityState.visibility
      : getDefaultColumnVisibility(logCategory)

  useEffect(() => {
    if (columnVisibilityState.logCategory === logCategory) {
      return
    }
    setColumnVisibilityState({
      logCategory,
      visibility: readColumnVisibility(logCategory),
    })
  }, [columnVisibilityState.logCategory, logCategory])

  useEffect(() => {
    if (columnVisibilityState.logCategory !== logCategory) {
      return
    }
    writeColumnVisibility(logCategory, columnVisibilityState.visibility)
  }, [columnVisibilityState, logCategory])

  const handleColumnVisibilityChange: OnChangeFn<VisibilityState> = (
    updater
  ) => {
    setColumnVisibilityState((prev) => {
      const base =
        prev.logCategory === logCategory
          ? prev.visibility
          : getDefaultColumnVisibility(logCategory)
      const nextVisibility =
        typeof updater === 'function' ? updater(base) : updater

      return {
        logCategory,
        visibility: nextVisibility,
      }
    })
  }

  const { table } = useDataTable({
    data: logs as Record<string, unknown>[],
    columns: columns as ColumnDef<Record<string, unknown>>[],
    columnFilters,
    columnVisibility,
    pagination,
    enableRowSelection: false,
    onPaginationChange,
    onColumnFiltersChange,
    onColumnVisibilityChange: handleColumnVisibilityChange,
    manualPagination: true,
    manualFiltering: true,
    totalCount: data?.total || 0,
    ensurePageInRange,
  })

  const isCommon = logCategory === 'common'

  return (
    <DataTablePage
      table={table}
      columns={columns as ColumnDef<Record<string, unknown>>[]}
      isLoading={isLoadingData}
      isFetching={isFetching}
      emptyTitle={t('No Logs Found')}
      emptyDescription={t(
        'No usage logs available. Logs will appear here once API calls are made.'
      )}
      skeletonKeyPrefix='usage-log-skeleton'
      applyHeaderSize
      tableClassName={cn(
        '[&_[data-slot=table]]:text-[13px] [&_[data-slot=table]_td]:text-[13px] [&_[data-slot=table]_td_*]:text-[13px] [&_[data-slot=table]_th]:text-[13px] [&_[data-slot=table]_th_*]:text-[13px]'
      )}
      mobile={
        <UsageLogsMobileList
          table={table}
          isLoading={isLoadingData}
          logCategory={logCategory}
        />
      }
      toolbar={
        isCommon ? (
          <CommonLogsFilterBar table={table} />
        ) : (
          <TaskLogsFilterBar table={table} logCategory={logCategory} />
        )
      }
      renderRow={(row) => {
        const logType = (row.original as Record<string, unknown>).type as
          | number
          | undefined
        const tintClass =
          isCommon && logType != null ? (logTypeRowTint[logType] ?? '') : ''

        return (
          <DataTableRow
            key={row.id}
            row={row}
            className={cn('transition-colors', tintClass)}
            getColumnClassName={() => (isCommon ? 'py-2' : 'py-3.5')}
          />
        )
      }}
    />
  )
}
