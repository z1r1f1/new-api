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
import { Check, Copy, Eye, Radio, Send, Zap } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { DEBUG_TABS } from '../constants'
import type { PlaygroundDebugData, PlaygroundDebugTab } from '../types'

interface PlaygroundDebugPanelProps {
  debugData: PlaygroundDebugData
  activeDebugTab: PlaygroundDebugTab
  customRequestMode: boolean
  onActiveDebugTabChange: (tab: PlaygroundDebugTab) => void
}

function formatDebugValue(value: unknown): string {
  if (value === null || value === undefined || value === '') {
    return ''
  }
  if (typeof value === 'string') {
    const trimmed = value.trim()
    if (!trimmed) return ''
    try {
      return JSON.stringify(JSON.parse(trimmed), null, 2)
    } catch {
      return value
    }
  }
  return JSON.stringify(value, null, 2)
}

function DebugCodeBlock(props: { value: unknown; emptyText: string }) {
  const { t } = useTranslation()
  const { copiedText, copyToClipboard } = useCopyToClipboard()
  const formatted = formatDebugValue(props.value)
  const isCopied = copiedText === formatted && formatted !== ''

  return (
    <div className='relative h-full min-h-0'>
      {formatted ? (
        <>
          <Button
            size='icon-sm'
            variant='ghost'
            className='absolute top-2 right-2 z-10 bg-background/80 backdrop-blur'
            onClick={() => copyToClipboard(formatted)}
            aria-label={t('Copy')}
          >
            {isCopied ? <Check className='size-4' /> : <Copy className='size-4' />}
          </Button>
          <ScrollArea className='h-full rounded-lg border bg-muted/40'>
            <pre className='p-3 pr-11 font-mono text-xs whitespace-pre-wrap break-words'>
              {formatted}
            </pre>
          </ScrollArea>
        </>
      ) : (
        <div className='text-muted-foreground flex h-full min-h-48 items-center justify-center rounded-lg border border-dashed p-4 text-center text-sm'>
          {props.emptyText}
        </div>
      )}
    </div>
  )
}

export function PlaygroundDebugPanel(props: PlaygroundDebugPanelProps) {
  const { t } = useTranslation()
  const sseCount = props.debugData.sseMessages.length
  const hasPreview = props.debugData.previewRequest !== null
  const hasRequest =
    props.debugData.gatewayRequest !== null ||
    props.debugData.upstreamRequest !== null ||
    props.debugData.request !== null
  const hasResponse = props.debugData.response !== null

  return (
    <div className='flex h-full flex-col overflow-hidden p-4'>
      <Card className='h-full min-h-0' size='sm'>
        <CardHeader className='shrink-0'>
          <CardTitle className='flex items-center gap-2'>
            <Zap className='size-4' />
            {t('Debug Information')}
            {props.customRequestMode && (
              <Badge variant='secondary'>{t('Custom')}</Badge>
            )}
          </CardTitle>
          <CardDescription>
            {t(
              'Inspect preview, gateway request, upstream request, response, and raw SSE data.'
            )}
          </CardDescription>
        </CardHeader>
        <CardContent className='flex min-h-0 flex-1 flex-col'>
          <Tabs
            value={props.activeDebugTab}
            onValueChange={(value) =>
              props.onActiveDebugTabChange(value as PlaygroundDebugTab)
            }
            className='min-h-0 flex-1'
          >
            <TabsList className='w-full'>
              <TabsTrigger value={DEBUG_TABS.PREVIEW} className='flex-1'>
                <Eye className='size-4' />
                {t('Preview')}
                {hasPreview && <Badge variant='outline'>●</Badge>}
              </TabsTrigger>
              <TabsTrigger value={DEBUG_TABS.REQUEST} className='flex-1'>
                <Send className='size-4' />
                {t('Request')}
                {hasRequest && <Badge variant='outline'>●</Badge>}
              </TabsTrigger>
              <TabsTrigger value={DEBUG_TABS.RESPONSE} className='flex-1'>
                <Radio className='size-4' />
                {t('Response')}
                {sseCount > 0 && (
                  <Badge variant='outline'>{sseCount}</Badge>
                )}
                {hasResponse && sseCount === 0 && (
                  <Badge variant='outline'>●</Badge>
                )}
              </TabsTrigger>
            </TabsList>

            <TabsContent value={DEBUG_TABS.PREVIEW} className='min-h-0 flex-1'>
              <DebugCodeBlock
                value={props.debugData.previewRequest}
                emptyText={t('No preview request yet.')}
              />
            </TabsContent>
            <TabsContent value={DEBUG_TABS.REQUEST} className='min-h-0 flex-1'>
              <div className='grid h-full min-h-0 grid-rows-2 gap-3'>
                <div className='min-h-0'>
                  <div className='mb-2 flex items-center justify-between'>
                    <p className='text-sm font-medium'>
                      {t('Gateway Request')}
                    </p>
                  </div>
                  <DebugCodeBlock
                    value={
                      props.debugData.gatewayRequest ?? props.debugData.request
                    }
                    emptyText={t('No request has been sent yet.')}
                  />
                </div>
                <div className='min-h-0'>
                  <div className='mb-2 flex items-center justify-between'>
                    <p className='text-sm font-medium'>
                      {t('Upstream Request')}
                    </p>
                    {props.debugData.upstreamRequest !== null && (
                      <Badge variant='secondary'>{t('Captured')}</Badge>
                    )}
                  </div>
                  <DebugCodeBlock
                    value={props.debugData.upstreamRequest}
                    emptyText={t('Waiting for upstream request capture.')}
                  />
                </div>
              </div>
            </TabsContent>
            <TabsContent value={DEBUG_TABS.RESPONSE} className='min-h-0 flex-1'>
              <div className='grid h-full min-h-0 gap-3'>
                {sseCount > 0 && (
                  <div className='min-h-0'>
                    <div className='mb-2 flex items-center justify-between'>
                      <p className='text-sm font-medium'>{t('Raw SSE')}</p>
                      {props.debugData.isStreaming && (
                        <Badge variant='secondary'>{t('Streaming')}</Badge>
                      )}
                    </div>
                    <DebugCodeBlock
                      value={props.debugData.sseMessages.join('\n')}
                      emptyText={t('No SSE events captured yet.')}
                    />
                  </div>
                )}
                <div className='min-h-0'>
                  <div className='mb-2 flex items-center justify-between'>
                    <p className='text-sm font-medium'>{t('Response')}</p>
                    {props.debugData.timestamp && (
                      <span className='text-muted-foreground text-xs'>
                        {new Date(props.debugData.timestamp).toLocaleString()}
                      </span>
                    )}
                  </div>
                  <DebugCodeBlock
                    value={props.debugData.response}
                    emptyText={t('No response yet.')}
                  />
                </div>
              </div>
            </TabsContent>
          </Tabs>
        </CardContent>
      </Card>
    </div>
  )
}
