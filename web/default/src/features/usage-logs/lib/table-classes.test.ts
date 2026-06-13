import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { cn } from '@/lib/utils'
import { getUsageLogTableCellClassName } from './table-classes'

describe('usage log table cell classes', () => {
  test('lets dense log cells wrap instead of inheriting global truncation', () => {
    const merged = cn(
      'truncate p-2 align-middle',
      getUsageLogTableCellClassName('common', 'cell')
    )

    assert.match(merged, /whitespace-normal/)
    assert.match(merged, /overflow-visible/)
    assert.match(merged, /break-words/)
    assert.doesNotMatch(merged, /truncate/)
    assert.doesNotMatch(merged, /whitespace-nowrap/)
  })

  test('clips long model names inside the model column', () => {
    const merged = cn(
      'truncate p-2 align-middle',
      getUsageLogTableCellClassName('common', 'cell', 'model_name')
    )

    assert.match(merged, /overflow-hidden/)
    assert.match(merged, /truncate/)
    assert.doesNotMatch(merged, /overflow-visible/)
    assert.doesNotMatch(merged, /whitespace-normal/)
  })

  test('keeps the compact vertical rhythm for each log category', () => {
    assert.match(getUsageLogTableCellClassName('common', 'cell') ?? '', /py-2/)
    assert.match(getUsageLogTableCellClassName('task', 'cell') ?? '', /py-3\.5/)
    assert.match(
      getUsageLogTableCellClassName('drawing', 'cell') ?? '',
      /py-3\.5/
    )
  })

  test('leaves header truncation unchanged', () => {
    assert.equal(getUsageLogTableCellClassName('common', 'header'), undefined)
  })
})
