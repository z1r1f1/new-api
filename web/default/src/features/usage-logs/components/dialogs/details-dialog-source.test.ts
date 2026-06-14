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
import { readFileSync } from 'node:fs'
import { describe, test } from 'node:test'
import { fileURLToPath } from 'node:url'

const sourcePath = fileURLToPath(
  new URL('./details-dialog.tsx', import.meta.url)
)

describe('usage log details dialog source contract', () => {
  test('renders safe admin request headers in log details', () => {
    const source = readFileSync(sourcePath, 'utf8')

    assert.match(source, /getVisibleAdminRequestHeaders/)
    assert.match(source, /requestHeaderRows/)
    assert.match(source, /Request Headers/)
  })

  test('lets the shared dialog body own scrolling for long details', () => {
    const source = readFileSync(sourcePath, 'utf8')

    assert.doesNotMatch(source, /ScrollArea/)
    assert.match(source, /contentHeight='min\(72vh, 720px\)'/)
  })
})
