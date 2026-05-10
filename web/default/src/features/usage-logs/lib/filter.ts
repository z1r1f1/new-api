/**
 * Utility functions for usage logs filters
 */
import { LOG_CATEGORY_LABELS } from '../constants'
import type {
  LogCategory,
  LogFilters,
  CommonLogFilters,
  DrawingLogFilters,
  TaskLogFilters,
} from '../types'

// ============================================================================
// Filter Building Functions
// ============================================================================

/**
 * Build search params from filters based on log category
 */
export function buildSearchParams(
  filters: LogFilters,
  logCategory: LogCategory
): Record<string, unknown> {
  const baseParams: Record<string, unknown> = {
    ...(filters.startTime && { startTime: filters.startTime.getTime() }),
    ...(filters.endTime && { endTime: filters.endTime.getTime() }),
  }

  switch (logCategory) {
    case 'common': {
      const commonFilters = filters as CommonLogFilters
      return {
        ...baseParams,
        ...(commonFilters.channelId && { channelId: commonFilters.channelId }),
        ...(commonFilters.channelName && {
          channelName: commonFilters.channelName,
        }),
        ...(commonFilters.model && { model: commonFilters.model }),
        ...(commonFilters.token && { token: commonFilters.token }),
        ...(commonFilters.group && { group: commonFilters.group }),
        ...(commonFilters.username && { username: commonFilters.username }),
        ...(commonFilters.requestId && { requestId: commonFilters.requestId }),
      }
    }
    case 'drawing': {
      const drawingFilters = filters as DrawingLogFilters
      return {
        ...baseParams,
        ...(drawingFilters.channel && { channel: drawingFilters.channel }),
        ...(drawingFilters.mjId && { filter: drawingFilters.mjId }),
      }
    }
    case 'task': {
      const taskFilters = filters as TaskLogFilters
      return {
        ...baseParams,
        ...(taskFilters.channel && { channel: taskFilters.channel }),
        ...(taskFilters.taskId && { filter: taskFilters.taskId }),
      }
    }
    default:
      return baseParams
  }
}

/**
 * Get log category display name
 */
export function getLogCategoryLabel(category: LogCategory): string {
  return LOG_CATEGORY_LABELS[category]
}
