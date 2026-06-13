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

export function getUsageLogTableCellClassName(
  logCategory: LogCategory,
  kind: 'header' | 'cell'
): string | undefined {
  if (kind !== 'cell') {
    return undefined
  }

  return logCategory === 'common'
    ? `py-2 ${USAGE_LOG_CELL_TEXT_FLOW_CLASS}`
    : `py-3.5 ${USAGE_LOG_CELL_TEXT_FLOW_CLASS}`
}
