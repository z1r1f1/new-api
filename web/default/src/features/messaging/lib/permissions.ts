import { ROLE } from '@/lib/roles'
import type { ChatConversation } from '../types'

interface ConversationMemberMuteState {
  isGroupMember: boolean
  muted: boolean
}

export function canManageChatProfileUser(
  currentUserRole: number,
  targetUserRole: number
): boolean {
  if (currentUserRole === ROLE.SUPER_ADMIN) return true
  return currentUserRole >= ROLE.ADMIN && currentUserRole > targetUserRole
}

export function getConversationMemberMuteState(
  conversation: ChatConversation | null,
  userId: number
): ConversationMemberMuteState {
  if (!conversation || conversation.type !== 'group') {
    return { isGroupMember: false, muted: false }
  }
  const member = conversation.members.find((item) => item.id === userId)
  return {
    isGroupMember: Boolean(member),
    muted: member?.muted === true,
  }
}
