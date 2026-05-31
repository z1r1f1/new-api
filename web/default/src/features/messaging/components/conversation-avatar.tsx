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
import { cn } from '@/lib/utils'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { getConversationInitial, getConversationTone } from '../lib/format'
import type { ChatConversation } from '../types'

interface ConversationAvatarProps {
  conversation: ChatConversation
  className?: string
}

export function ConversationAvatar(props: ConversationAvatarProps) {
  return (
    <Avatar size='lg' className={cn('shadow-sm', props.className)}>
      <AvatarFallback
        className={cn(
          'bg-gradient-to-br text-white',
          getConversationTone(props.conversation)
        )}
      >
        {getConversationInitial(props.conversation)}
      </AvatarFallback>
    </Avatar>
  )
}
