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
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import {
  WebPreview,
  WebPreviewBody,
  WebPreviewConsole,
} from '../ai-elements/web-preview'
import { DataTableFacetedFilter } from '../data-table/toolbar/faceted-filter'
import { RiskAcknowledgementDialog } from '../risk-acknowledgement-dialog'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

it('preserves repeated preview log entries without duplicate identities', async () => {
  const errors = vi.spyOn(console, 'error').mockImplementation(() => undefined)
  const log = {
    level: 'log' as const,
    message: 'Repeated message',
    timestamp: new Date('2026-01-01T00:00:00Z'),
  }
  const user = userEvent.setup()
  const { rerender } = render(
    <WebPreview>
      <WebPreviewConsole logs={[log, log]} />
    </WebPreview>
  )
  await user.click(screen.getByRole('button', { name: 'Console' }))
  expect(screen.getAllByText('Repeated message')).toHaveLength(2)
  rerender(
    <WebPreview>
      <WebPreviewConsole logs={[log]} />
    </WebPreview>
  )
  expect(screen.getAllByText('Repeated message')).toHaveLength(1)
  expect(errors.mock.calls.flat().join(' ')).not.toContain('same key')
})

it('requires every acknowledgement input even with repeated empty static parts', async () => {
  const errors = vi.spyOn(console, 'error').mockImplementation(() => undefined)
  const confirm = vi.fn()
  const user = userEvent.setup()
  render(
    <RiskAcknowledgementDialog
      open
      onOpenChange={() => undefined}
      title='Confirm action'
      description='Complete both fields'
      requiredTextParts={[
        { type: 'static', text: '' },
        { type: 'static', text: '' },
        { type: 'input', text: 'YES' },
        { type: 'static', text: '+' },
        { type: 'input', text: 'YES' },
      ]}
      onConfirm={confirm}
    />
  )
  const inputs = screen.getAllByRole('textbox')
  const button = screen.getByRole('button', { name: 'Confirm' })
  expect(button).toBeDisabled()
  await user.type(inputs[0], 'YES')
  expect(button).toBeDisabled()
  await user.type(inputs[1], 'YES')
  expect(button).toBeEnabled()
  await user.click(button)
  expect(confirm).toHaveBeenCalledOnce()
  expect(errors.mock.calls.flat().join(' ')).not.toContain('same key')
})

it('does not render a numeric icon placeholder as visible filter text', async () => {
  const user = userEvent.setup()
  render(
    <DataTableFacetedFilter
      title='Status'
      options={[{ value: 'enabled', label: 'Enabled', iconNode: 0, count: 0 }]}
    />
  )
  await user.click(screen.getByRole('button', { name: 'Status' }))
  const option = await screen.findByRole('option')
  expect(option.textContent).toBe('Enabled0')
})

it('allows preview scripts without granting the embedded document same-origin privileges', () => {
  render(
    <WebPreview defaultUrl='https://preview.example/'>
      <WebPreviewBody />
    </WebPreview>
  )
  const frame = screen.getByTitle('Preview')
  expect(frame).toHaveAttribute('src', 'https://preview.example/')
  const permissions = frame.getAttribute('sandbox')?.split(' ')
  expect(permissions).toContain('allow-scripts')
  expect(permissions).not.toContain('allow-same-origin')
})
