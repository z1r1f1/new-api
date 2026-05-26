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
import { useQueryClient } from '@tanstack/react-query'
import { Loader2, RefreshCw, DollarSign, ImageIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { formatCurrencyFromUSD } from '@/lib/currency'
import { formatTimestampToDate } from '@/lib/format'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { getCodexUsage, updateChannelBalance } from '../../api'
import { channelsQueryKeys } from '../../lib'
import type { ChannelBalanceResponse } from '../../types'
import { useChannels } from '../channels-provider'
import {
  CodexUsageDialog,
  type CodexUsageDialogData,
} from './codex-usage-dialog'

type BalanceQueryDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

type ChatGPTImageQuotaData = NonNullable<ChannelBalanceResponse['data']>

export function BalanceQueryDialog({
  open,
  onOpenChange,
}: BalanceQueryDialogProps) {
  const { t } = useTranslation()
  const { currentRow, setCurrentRow } = useChannels()
  const queryClient = useQueryClient()
  const [isQuerying, setIsQuerying] = useState(false)
  const [balance, setBalance] = useState<number | null>(null)
  const [balanceUpdatedTime, setBalanceUpdatedTime] = useState<number | null>(
    null
  )
  const [codexUsageResponse, setCodexUsageResponse] =
    useState<CodexUsageDialogData | null>(null)
  const [imageQuotaData, setImageQuotaData] =
    useState<ChatGPTImageQuotaData | null>(null)

  const isCodex = currentRow?.type === 57
  const isChatGPTWeb = currentRow?.type === 58

  const handleQueryCodexUsage = async () => {
    const row = currentRow
    if (!row) return
    setIsQuerying(true)
    try {
      const res = await getCodexUsage(row.id)
      if (!res.success) {
        throw new Error(res.message || t('Failed to fetch usage'))
      }
      setCodexUsageResponse(res)
    } catch (error: unknown) {
      toast.error(
        error instanceof Error ? error.message : t('Failed to fetch usage')
      )
    } finally {
      setIsQuerying(false)
    }
  }

  useEffect(() => {
    if (!isCodex) return
    if (!open) return
    handleQueryCodexUsage()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, isCodex])

  if (!currentRow) return null

  const handleQueryBalance = async () => {
    setIsQuerying(true)
    try {
      const response = await updateChannelBalance(currentRow.id)
      if (response.success && response.balance !== undefined) {
        const newBalance = response.balance
        const now = Math.floor(Date.now() / 1000)

        setBalance(newBalance)
        setBalanceUpdatedTime(now)
        setImageQuotaData(response.data ?? null)
        if (isChatGPTWeb) {
          toast.success(
            t('Image quota updated: {{quota}}', {
              quota: formatImageQuota(
                response.data?.image_quota_remaining ?? newBalance,
                response.data?.image_quota_total
              ),
            })
          )
        } else {
          toast.success(t('Balance updated successfully'))
        }

        // Update currentRow immediately with new balance and timestamp
        setCurrentRow({
          ...currentRow,
          balance: newBalance,
          balance_updated_time: now,
        })

        // Invalidate queries to refresh the table
        await queryClient.invalidateQueries({
          queryKey: channelsQueryKeys.lists(),
        })
      } else {
        toast.error(response.message || t('Failed to query balance'))
      }
    } catch (error: unknown) {
      toast.error(
        error instanceof Error ? error.message : t('Failed to query balance')
      )
    } finally {
      setIsQuerying(false)
    }
  }

  const handleClose = () => {
    setBalance(null)
    setBalanceUpdatedTime(null)
    setCodexUsageResponse(null)
    setImageQuotaData(null)
    onOpenChange(false)
  }

  const formatBalance = (bal: number) =>
    formatCurrencyFromUSD(bal, {
      digitsLarge: 2,
      digitsSmall: 4,
      abbreviate: false,
    })

  const formatDate = (timestamp: number) => {
    if (!timestamp) return t('Never')
    return formatTimestampToDate(timestamp)
  }

  const formatImageCount = (value: number | null | undefined) => {
    if (value == null || Number.isNaN(value)) return '-'
    return String(Math.max(0, Math.trunc(value)))
  }

  const formatImageQuota = (
    remaining: number | null | undefined,
    total: number | null | undefined
  ) => {
    const remainingText = formatImageCount(remaining)
    if (remainingText === '-') return remainingText
    if (total == null || total <= 0 || Number.isNaN(total)) {
      return remainingText
    }
    return `${remainingText}/${formatImageCount(total)}`
  }

  const formatImageQuotaWindow = (window: string | null | undefined) => {
    const normalized = window?.trim().toLowerCase()
    if (normalized === 'daily') return t('Likely daily')
    if (normalized === 'weekly') return t('Likely weekly')
    if (normalized === 'monthly') return t('Likely monthly')
    if (normalized === 'resetting_soon') return t('Resetting soon')
    return t('Unknown')
  }

  const formatResetCountdown = (
    resetAt: number | null | undefined,
    resetAfterSeconds: number | null | undefined
  ) => {
    if (!resetAt && resetAfterSeconds == null) return '-'

    let seconds = Number(resetAfterSeconds)
    if (!Number.isFinite(seconds) || seconds <= 0) {
      seconds = resetAt
        ? Math.floor(resetAt - Math.floor(Date.now() / 1000))
        : 0
    }
    seconds = Math.max(0, Math.trunc(seconds))
    if (seconds <= 0) return t('Resetting soon')

    const days = Math.floor(seconds / 86400)
    const hours = Math.floor((seconds % 86400) / 3600)
    const minutes = Math.floor((seconds % 3600) / 60)

    if (days > 0) {
      return `${days} ${t('days')} ${hours} ${t('hours')}`
    }
    if (hours > 0) {
      return `${hours} ${t('hours')} ${minutes} ${t('minutes')}`
    }
    return `${Math.max(1, minutes)} ${t('minutes')}`
  }

  let currentValueDisplay = formatBalance(currentRow.balance)
  if (balance !== null) {
    currentValueDisplay = formatBalance(balance)
  }
  if (isChatGPTWeb) {
    currentValueDisplay = formatImageQuota(
      imageQuotaData?.image_quota_remaining ?? balance ?? currentRow.balance,
      imageQuotaData?.image_quota_total
    )
  }

  let updateButtonText = t('Update Balance')
  if (isQuerying) {
    updateButtonText = t('Querying...')
  } else if (isChatGPTWeb) {
    updateButtonText = t('Update Image Quota')
  }

  if (isCodex) {
    return (
      <CodexUsageDialog
        open={open}
        onOpenChange={(v) => {
          if (!v) handleClose()
        }}
        channelName={currentRow.name}
        channelId={currentRow.id}
        response={codexUsageResponse}
        onRefresh={handleQueryCodexUsage}
        isRefreshing={isQuerying}
      />
    )
  }

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {isChatGPTWeb ? t('Query Image Quota') : t('Query Balance')}
          </DialogTitle>
          <DialogDescription>
            {t('Update balance for:')} <strong>{currentRow.name}</strong>
          </DialogDescription>
        </DialogHeader>

        <div className='space-y-4 py-4'>
          {/* Current Balance Display */}
          <div className='bg-muted/50 rounded-lg border p-4'>
            <div className='text-muted-foreground mb-2 flex items-center gap-2 text-sm'>
              {isChatGPTWeb ? (
                <ImageIcon className='h-4 w-4' />
              ) : (
                <DollarSign className='h-4 w-4' />
              )}
              <span>
                {isChatGPTWeb ? t('Current Image Quota') : t('Current Balance')}
              </span>
            </div>
            <div className='text-2xl font-bold'>{currentValueDisplay}</div>
            <div className='text-muted-foreground mt-2 text-xs'>
              {t('Last updated:')}{' '}
              {formatDate(
                balanceUpdatedTime ?? currentRow.balance_updated_time
              )}
            </div>
          </div>

          {isChatGPTWeb && (
            <div className='bg-muted/30 space-y-2 rounded-lg border p-4 text-sm'>
              <div className='flex items-center justify-between gap-4'>
                <span className='text-muted-foreground'>
                  {t('Default model')}
                </span>
                <span className='text-right font-medium'>
                  {imageQuotaData?.default_model_slug || '-'}
                </span>
              </div>
              <div className='flex items-center justify-between gap-4'>
                <span className='text-muted-foreground'>
                  {t('Image quota reset time')}
                </span>
                <span className='text-right font-medium'>
                  {formatDate(imageQuotaData?.image_quota_reset_at ?? 0)}
                </span>
              </div>
              <div className='flex items-center justify-between gap-4'>
                <span className='text-muted-foreground'>
                  {t('Estimated quota period')}
                </span>
                <span className='text-right font-medium'>
                  {formatImageQuotaWindow(imageQuotaData?.image_quota_window)}
                </span>
              </div>
              <div className='flex items-center justify-between gap-4'>
                <span className='text-muted-foreground'>
                  {t('Reset countdown')}
                </span>
                <span className='text-right font-medium'>
                  {formatResetCountdown(
                    imageQuotaData?.image_quota_reset_at,
                    imageQuotaData?.image_quota_reset_after_seconds
                  )}
                </span>
              </div>
              <div className='flex items-start justify-between gap-4'>
                <span className='text-muted-foreground'>
                  {t('Blocked features')}
                </span>
                <span className='text-right font-medium break-all'>
                  {imageQuotaData?.blocked_features?.length
                    ? imageQuotaData.blocked_features.join(', ')
                    : t('None')}
                </span>
              </div>
              <p className='text-muted-foreground border-t pt-2 text-xs leading-relaxed'>
                {t(
                  'Quota period is estimated from the upstream reset time because ChatGPT Web does not label the window directly.'
                )}
              </p>
            </div>
          )}

          {/* Balance Update Button */}
          <Button
            className='w-full'
            onClick={handleQueryBalance}
            disabled={isQuerying}
          >
            {isQuerying && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
            {!isQuerying && <RefreshCw className='mr-2 h-4 w-4' />}
            {updateButtonText}
          </Button>
        </div>

        <DialogFooter>
          <Button variant='outline' onClick={handleClose} disabled={isQuerying}>
            {t('Close')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
