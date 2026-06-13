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
import type { LogCategory } from '../types'

const USAGE_LOG_CELL_TEXT_FLOW_CLASS =
  'whitespace-normal overflow-visible text-clip break-words'

const USAGE_LOG_CLIPPED_CELL_CLASS = 'overflow-hidden'

export function getUsageLogTableCellClassName(
  logCategory: LogCategory,
  kind: 'header' | 'cell',
  columnId?: string
): string | undefined {
  if (kind !== 'cell') {
    return undefined
  }

  const spacingClass = logCategory === 'common' ? 'py-2' : 'py-3.5'

  if (columnId === 'model_name') {
    return `${spacingClass} ${USAGE_LOG_CLIPPED_CELL_CLASS}`
  }

  return `${spacingClass} ${USAGE_LOG_CELL_TEXT_FLOW_CLASS}`
}
