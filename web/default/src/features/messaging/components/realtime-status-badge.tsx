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
import { Loader2, Wifi, WifiOff } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { getRealtimeBadgeVariant } from '../lib/format'

interface RealtimeStatusBadgeProps {
  status: string
}

export function RealtimeStatusBadge(props: RealtimeStatusBadgeProps) {
  const { t } = useTranslation()
  return (
    <Badge
      variant={getRealtimeBadgeVariant(props.status)}
      className='gap-1 rounded-full'
    >
      <RealtimeStatusIcon status={props.status} />
      {t(props.status)}
    </Badge>
  )
}

function RealtimeStatusIcon(props: RealtimeStatusBadgeProps) {
  if (props.status === 'connected') return <Wifi className='h-3.5 w-3.5' />
  if (props.status === 'connecting') {
    return <Loader2 className='h-3.5 w-3.5 animate-spin' />
  }
  return <WifiOff className='h-3.5 w-3.5' />
}
