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
import { useRef } from 'react'
import {
  Download,
  FileInput,
  RotateCcw,
  Settings2,
  SlidersHorizontal,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { ModelGroupSelector } from '@/components/model-group-selector'
import { parseCustomRequestBody } from '../lib'
import type {
  GroupOption,
  Message,
  ModelOption,
  ParameterEnabled,
  PlaygroundConfig,
  PlaygroundWorkbenchState,
} from '../types'

interface PlaygroundSettingsPanelProps {
  config: PlaygroundConfig
  parameterEnabled: ParameterEnabled
  workbenchState: PlaygroundWorkbenchState
  models: ModelOption[]
  groups: GroupOption[]
  messages: Message[]
  disabled?: boolean
  onConfigChange: <K extends keyof PlaygroundConfig>(
    key: K,
    value: PlaygroundConfig[K]
  ) => void
  onParameterEnabledChange: (key: keyof ParameterEnabled, value: boolean) => void
  onWorkbenchChange: <K extends keyof PlaygroundWorkbenchState>(
    key: K,
    value: PlaygroundWorkbenchState[K]
  ) => void
  onExport: () => void
  onImport: (file: File) => void
  onReset: () => void
}

interface NumberParameterProps {
  label: string
  name: keyof ParameterEnabled
  value: number | null
  enabled: boolean
  min?: number
  max?: number
  step?: number
  disabled?: boolean
  onEnabledChange: (key: keyof ParameterEnabled, value: boolean) => void
  onValueChange: (value: number | null) => void
}

function NumberParameter(props: NumberParameterProps) {
  const inputValue = props.value === null ? '' : String(props.value)

  return (
    <div className='space-y-2 rounded-lg border p-3'>
      <div className='flex items-center justify-between gap-3'>
        <Label className='text-sm font-medium'>{props.label}</Label>
        <Switch
          checked={props.enabled}
          disabled={props.disabled}
          onCheckedChange={(checked) =>
            props.onEnabledChange(props.name, checked)
          }
          size='sm'
        />
      </div>
      <Input
        disabled={props.disabled || !props.enabled}
        min={props.min}
        max={props.max}
        step={props.step}
        type='number'
        value={inputValue}
        onChange={(event) => {
          const nextValue = event.target.value
          props.onValueChange(nextValue === '' ? null : Number(nextValue))
        }}
      />
    </div>
  )
}

export function PlaygroundSettingsPanel(props: PlaygroundSettingsPanelProps) {
  const { t } = useTranslation()
  const fileInputRef = useRef<HTMLInputElement | null>(null)
  const customValidation = parseCustomRequestBody(
    props.workbenchState.customRequestBody
  )
  const customMode = props.workbenchState.customRequestMode
  const controlsDisabled = props.disabled || customMode

  return (
    <div className='grid h-full auto-rows-max grid-cols-1 gap-4 overflow-y-auto overscroll-contain p-5 pb-8 lg:grid-cols-2'>
      <Card size='sm'>
        <CardHeader>
          <CardTitle className='flex items-center gap-2'>
            <Settings2 className='size-4' />
            {t('Playground Settings')}
          </CardTitle>
          <CardDescription>
            {t('Configure the request before sending it upstream.')}
          </CardDescription>
        </CardHeader>
        <CardContent className='space-y-4'>
          <div className={customMode ? 'opacity-60' : ''}>
            <Label className='mb-2 block'>{t('Model and group')}</Label>
            <ModelGroupSelector
              selectedModel={props.config.model}
              models={props.models}
              onModelChange={(value) => props.onConfigChange('model', value)}
              selectedGroup={props.config.group}
              groups={props.groups}
              onGroupChange={(value) => props.onConfigChange('group', value)}
              disabled={controlsDisabled}
              className='flex-wrap'
            />
            {customMode && (
              <p className='text-muted-foreground mt-2 text-xs'>
                {t('Ignored while custom request body mode is enabled.')}
              </p>
            )}
          </div>

          <div className={customMode ? 'opacity-60' : ''}>
            <div className='flex items-center justify-between gap-3 rounded-lg border p-3'>
              <div>
                <Label>{t('Stream output')}</Label>
                <p className='text-muted-foreground text-xs'>
                  {t('Use server-sent events for chat completions.')}
                </p>
              </div>
              <Switch
                checked={props.config.stream}
                disabled={controlsDisabled}
                onCheckedChange={(checked) =>
                  props.onConfigChange('stream', checked)
                }
              />
            </div>
          </div>
        </CardContent>
      </Card>

      <Card size='sm'>
        <CardHeader>
          <CardTitle className='flex items-center gap-2'>
            <SlidersHorizontal className='size-4' />
            {t('Request Parameters')}
          </CardTitle>
          <CardDescription>
            {t('Only enabled parameters are included in the request body.')}
          </CardDescription>
        </CardHeader>
        <CardContent className='space-y-3'>
          <NumberParameter
            label='temperature'
            name='temperature'
            value={props.config.temperature}
            enabled={props.parameterEnabled.temperature}
            min={0}
            max={2}
            step={0.1}
            disabled={controlsDisabled}
            onEnabledChange={props.onParameterEnabledChange}
            onValueChange={(value) =>
              props.onConfigChange('temperature', value ?? 0)
            }
          />
          <NumberParameter
            label='top_p'
            name='top_p'
            value={props.config.top_p}
            enabled={props.parameterEnabled.top_p}
            min={0}
            max={1}
            step={0.05}
            disabled={controlsDisabled}
            onEnabledChange={props.onParameterEnabledChange}
            onValueChange={(value) => props.onConfigChange('top_p', value ?? 1)}
          />
          <NumberParameter
            label='max_tokens'
            name='max_tokens'
            value={props.config.max_tokens}
            enabled={props.parameterEnabled.max_tokens}
            min={1}
            step={1}
            disabled={controlsDisabled}
            onEnabledChange={props.onParameterEnabledChange}
            onValueChange={(value) =>
              props.onConfigChange('max_tokens', value ?? 4096)
            }
          />
          <NumberParameter
            label='frequency_penalty'
            name='frequency_penalty'
            value={props.config.frequency_penalty}
            enabled={props.parameterEnabled.frequency_penalty}
            min={-2}
            max={2}
            step={0.1}
            disabled={controlsDisabled}
            onEnabledChange={props.onParameterEnabledChange}
            onValueChange={(value) =>
              props.onConfigChange('frequency_penalty', value ?? 0)
            }
          />
          <NumberParameter
            label='presence_penalty'
            name='presence_penalty'
            value={props.config.presence_penalty}
            enabled={props.parameterEnabled.presence_penalty}
            min={-2}
            max={2}
            step={0.1}
            disabled={controlsDisabled}
            onEnabledChange={props.onParameterEnabledChange}
            onValueChange={(value) =>
              props.onConfigChange('presence_penalty', value ?? 0)
            }
          />
          <NumberParameter
            label='seed'
            name='seed'
            value={props.config.seed}
            enabled={props.parameterEnabled.seed}
            min={0}
            step={1}
            disabled={controlsDisabled}
            onEnabledChange={props.onParameterEnabledChange}
            onValueChange={(value) => props.onConfigChange('seed', value)}
          />
        </CardContent>
      </Card>

      <Card size='sm'>
        <CardHeader>
          <CardTitle>{t('Custom Request Body')}</CardTitle>
          <CardDescription>
            {t('Send a raw JSON request body instead of the generated payload.')}
          </CardDescription>
        </CardHeader>
        <CardContent className='space-y-3'>
          <div className='flex items-center justify-between gap-3 rounded-lg border p-3'>
            <div>
              <Label>{t('Custom request body mode')}</Label>
              <p className='text-muted-foreground text-xs'>
                {t('When enabled, generated settings are ignored.')}
              </p>
            </div>
            <Switch
              checked={customMode}
              disabled={props.disabled}
              onCheckedChange={(checked) =>
                props.onWorkbenchChange('customRequestMode', checked)
              }
            />
          </div>

          {customMode && (
            <div className='space-y-2'>
              <Textarea
                className='h-64 min-h-64 resize-y font-mono text-xs'
                placeholder='{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}],"stream":true}'
                value={props.workbenchState.customRequestBody}
                onChange={(event) =>
                  props.onWorkbenchChange('customRequestBody', event.target.value)
                }
              />
              <p
                className={
                  customValidation.error
                    ? 'text-destructive text-xs'
                    : 'text-muted-foreground text-xs'
                }
              >
                {customValidation.error
                  ? t('JSON error: {{error}}', {
                      error: customValidation.error,
                    })
                  : t('JSON is valid.')}
              </p>
            </div>
          )}
        </CardContent>
      </Card>

      <Card size='sm'>
        <CardHeader>
          <CardTitle>{t('Configuration')}</CardTitle>
          <CardDescription>
            {t('Export, import, or reset Playground settings and messages.')}
          </CardDescription>
        </CardHeader>
        <CardContent className='space-y-2'>
          <input
            ref={fileInputRef}
            type='file'
            accept='application/json,.json'
            className='hidden'
            onChange={(event) => {
              const file = event.target.files?.[0]
              if (file) props.onImport(file)
              event.target.value = ''
            }}
          />
          <div className='grid grid-cols-1 gap-2 sm:grid-cols-3 lg:grid-cols-1'>
            <Button variant='outline' onClick={props.onExport}>
              <Download className='mr-2 size-4' />
              {t('Export')}
            </Button>
            <Button
              variant='outline'
              onClick={() => fileInputRef.current?.click()}
            >
              <FileInput className='mr-2 size-4' />
              {t('Import')}
            </Button>
            <Button variant='outline' onClick={props.onReset}>
              <RotateCcw className='mr-2 size-4' />
              {t('Reset')}
            </Button>
          </div>
          <p className='text-muted-foreground text-xs'>
            {t('{{count}} messages are included when exporting.', {
              count: props.messages.length,
            })}
          </p>
        </CardContent>
      </Card>
    </div>
  )
}
