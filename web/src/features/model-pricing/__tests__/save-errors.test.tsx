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
import { QueryClientProvider, type QueryClient } from '@tanstack/react-query'
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AxiosError, type AxiosAdapter } from 'axios'
import { useState } from 'react'
import { toast } from 'sonner'
import { afterEach, expect, it, vi } from 'vitest'

import { SettingsPageProvider } from '@/features/system-settings/components/settings-page-context'
import { RatioSettingsCard } from '@/features/system-settings/models/ratio-settings-card'
import { api } from '@/lib/http-client'
import { createAppQueryClient } from '@/lib/query-client'
import { useAuthStore } from '@/stores/auth-store'
import { usePricingPreferencesStore } from '@/stores/pricing-preferences-store'

import type { ModelPricingConfig } from '../api'
import { ModelPricingPanel } from '../model-pricing-panel'
import { pricingOptions } from '../pricing'

const originalAdapter = api.defaults.adapter
let client: QueryClient | undefined

afterEach(() => {
  cleanup()
  client?.clear()
  client = undefined
  api.defaults.adapter = originalAdapter
  useAuthStore.getState().auth.setUser(null)
  localStorage.clear()
  vi.restoreAllMocks()
})

it.each([200, 400])(
  'shows the backend pricing error once, preserves the draft, and reports a new attempt separately (HTTP %i)',
  async (status) => {
    useAuthStore
      .getState()
      .auth.setUser({ id: 1, username: 'administrator', role: 100 })
    usePricingPreferencesStore.setState({ currency: 'USD' })
    const values = {
      'billing_setting.billing_mode': 'tiered_expr',
      'billing_setting.billing_expr': 'tier("standard", p * 1 + c * 2)',
    }
    const snapshot: ModelPricingConfig = {
      entries: [
        {
          model_name: 'example',
          version: 'v1',
          configured: values,
          effective: values,
        },
      ],
      options: pricingOptions({}),
      empty_version: 'empty',
    }
    const message = 'model_pricing: expression validation failed (1:18)'
    const notify = vi.spyOn(toast, 'error').mockReturnValue('pricing-error')
    const requests: unknown[] = []
    const adapter: AxiosAdapter = async (config) => {
      if (
        config.method === 'get' &&
        ['/api/status', '/api/pricing'].includes(config.url ?? '')
      ) {
        const data =
          config.url === '/api/status'
            ? { success: true, data: {} }
            : { success: true, data: [], vendors: [] }
        return { data, status: 200, statusText: 'OK', headers: {}, config }
      }
      if (
        config.method === 'get' &&
        config.url === '/api/option/model_pricing'
      ) {
        return {
          data: { success: true, data: snapshot },
          status: 200,
          statusText: 'OK',
          headers: {},
          config,
        }
      }
      if (
        config.method === 'patch' &&
        config.url === '/api/option/model_pricing'
      ) {
        requests.push(config.data)
        const response = {
          data: { success: false, message },
          status,
          statusText: 'Error',
          headers: {},
          config,
        }
        if (status === 400) {
          throw new AxiosError(
            'Request failed with status code 400',
            'ERR_BAD_REQUEST',
            config,
            undefined,
            response
          )
        }
        return response
      }
      throw new Error(`Unexpected request: ${config.method} ${config.url}`)
    }
    api.defaults.adapter = adapter
    client = createAppQueryClient()
    render(
      <QueryClientProvider client={client}>
        <ModelPricingPanel modelName='example' />
      </QueryClientProvider>
    )
    const user = userEvent.setup()
    const price = await screen.findByRole('textbox', {
      name: 'Input price',
    })
    await user.clear(price)
    await user.type(price, '3')
    const save = screen.getByRole('button', { name: 'Save model prices' })
    await user.click(save)
    await waitFor(() =>
      expect(notify.mock.calls.map(([text]) => text)).toEqual([message])
    )
    expect(await screen.findByRole('alert')).toHaveTextContent(message)
    expect(price).toHaveValue('3')
    await waitFor(() => expect(save).toBeEnabled())
    await user.click(save)
    await waitFor(() =>
      expect(notify.mock.calls.map(([text]) => text)).toEqual([
        message,
        message,
      ])
    )
    expect(requests).toHaveLength(2)
    expect(requests[0]).toContain('p * 3')
  }
)

const settingsDefaults = {
  ModelPrice: '{"example":0.04}',
  ModelRatio: '{}',
  CacheRatio: '{}',
  CreateCacheRatio: '{}',
  CompletionRatio: '{}',
  ImageRatio: '{}',
  AudioRatio: '{}',
  AudioCompletionRatio: '{}',
  BillingMode: '{}',
  BillingExpr: '{}',
  PluginBillingExpr: JSON.stringify({
    'provider::example': 'tier("provider", 0.000000000001)',
  }),
  ExposeRatioEnabled: false,
}

function PricingSettingsFixture() {
  const [actions, setActions] = useState<HTMLDivElement | null>(null)
  return (
    <SettingsPageProvider actionsContainer={actions}>
      <header>
        <div ref={setActions} />
      </header>
      <RatioSettingsCard
        modelDefaults={settingsDefaults}
        groupDefaults={{
          GroupRatio: '{}',
          TopupGroupRatio: '{}',
          UserUsableGroups: '{}',
          GroupGroupRatio: '{}',
          AutoGroups: '[]',
          MaxTokenAutoGroups: 1,
          DefaultUseAutoGroup: false,
          GroupSpecialUsableGroup: '{}',
        }}
        toolPricesDefault='{}'
        visibleTabs={['models']}
      />
    </SettingsPageProvider>
  )
}

it.each([200, 400])(
  'retains the settings comparison and draft with one toast per failed request (HTTP %i)',
  async (status) => {
    useAuthStore
      .getState()
      .auth.setUser({ id: 1, username: 'administrator', role: 100 })
    usePricingPreferencesStore.setState({ currency: 'USD' })
    const providerExpression = 'tier("provider", 0.000000000001)'
    const values = {
      ModelPrice: 0.04,
      'billing_setting.plugin_billing_expr': { provider: providerExpression },
    }
    const snapshot: ModelPricingConfig = {
      entries: [
        {
          model_name: 'example',
          version: 'v1',
          configured: values,
          effective: values,
        },
      ],
      options: pricingOptions(settingsDefaults),
      empty_version: 'empty',
    }
    const message = 'model_pricing: pricing changed; reload before saving'
    const notify = vi
      .spyOn(toast, 'error')
      .mockReturnValue('settings-pricing-error')
    const requests: string[] = []
    const adapter: AxiosAdapter = async (config) => {
      if (config.method === 'get') {
        let data: unknown
        if (config.url === '/api/option/model_pricing') {
          data = { success: true, data: snapshot }
        } else if (config.url === '/api/status') {
          data = { success: true, data: {} }
        } else if (
          [
            '/api/pricing',
            '/api/task_plugin_options',
            '/api/option/model_pricing/issues',
          ].includes(config.url ?? '')
        ) {
          data = { success: true, data: [], vendors: [] }
        } else throw new Error(`Unexpected GET ${config.url}`)
        return { data, status: 200, statusText: 'OK', headers: {}, config }
      }
      if (
        config.method === 'patch' &&
        config.url === '/api/option/model_pricing'
      ) {
        requests.push(String(config.data))
        const response = {
          data: { success: false, message },
          status,
          statusText: 'Error',
          headers: {},
          config,
        }
        if (status === 400) {
          throw new AxiosError(
            'Request failed with status code 400',
            'ERR_BAD_REQUEST',
            config,
            undefined,
            response
          )
        }
        return response
      }
      throw new Error(`Unexpected request: ${config.method} ${config.url}`)
    }
    api.defaults.adapter = adapter
    client = createAppQueryClient()
    render(
      <QueryClientProvider client={client}>
        <PricingSettingsFixture />
      </QueryClientProvider>
    )
    const user = userEvent.setup()
    await waitFor(() => expect(client?.isFetching()).toBe(0))
    await user.click(await screen.findByRole('button', { name: 'Edit' }))
    const price = await screen.findByRole('textbox', { name: 'Fixed price' })
    await user.clear(price)
    await user.type(price, '0')
    const save = screen
      .getAllByRole('button', { name: /Save model prices/ })
      .find((button) => !button.closest('form'))
    if (!save) throw new Error('Page save action not found')
    await user.click(save)
    const dialog = await screen.findByRole('alertdialog', {
      name: 'Confirm price changes',
    })
    const confirm = within(dialog).getByRole('button', {
      name: 'Confirm and save',
    })
    await user.click(confirm)
    await waitFor(() =>
      expect(notify.mock.calls.map(([text]) => text)).toEqual([message])
    )
    expect(dialog).toBeVisible()
    expect(within(dialog).getByRole('row', { name: /example/ })).toBeVisible()
    await waitFor(() => expect(confirm).toBeEnabled())
    await user.click(confirm)
    await waitFor(() =>
      expect(notify.mock.calls.map(([text]) => text)).toEqual([
        message,
        message,
      ])
    )
    expect(dialog).toBeVisible()
    expect(requests).toHaveLength(2)
    expect(requests[1]).toBe(requests[0])
    expect(JSON.parse(requests[0])).toEqual({
      changes: [
        {
          model_name: 'example',
          expected_version: 'v1',
          pricing: {
            ModelPrice: 0,
            'billing_setting.billing_mode': 'ratio',
            'billing_setting.plugin_billing_expr': {
              provider: providerExpression,
            },
          },
        },
      ],
    })
    await user.keyboard('{Escape}')
    await waitFor(() =>
      expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
    )
  }
)
