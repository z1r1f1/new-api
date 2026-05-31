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
import {
  Loader2,
  MessageCircle,
  MessageSquarePlus,
  Plus,
  Users,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'

interface StartConversationPanelProps {
  directUserId: string
  groupTitle: string
  groupMemberIds: string
  creatingDirect: boolean
  creatingGroup: boolean
  onDirectUserIdChange: (value: string) => void
  onGroupTitleChange: (value: string) => void
  onGroupMemberIdsChange: (value: string) => void
  onCreateDirect: () => void
  onCreateGroup: () => void
}

export function StartConversationPanel(props: StartConversationPanelProps) {
  const { t } = useTranslation()

  return (
    <Card className='border-border/80 bg-background/95 shadow-sm'>
      <CardHeader className='pb-3'>
        <CardTitle className='flex items-center gap-2 text-sm'>
          <MessageSquarePlus className='h-4 w-4' />
          {t('Start conversation')}
        </CardTitle>
      </CardHeader>
      <CardContent className='space-y-4'>
        <div className='bg-muted/20 rounded-2xl border p-3'>
          <div className='text-muted-foreground mb-2 flex items-center gap-2 text-xs font-medium tracking-wide uppercase'>
            <MessageCircle className='h-3.5 w-3.5' />
            {t('Direct message')}
          </div>
          <div className='flex gap-2'>
            <Input
              value={props.directUserId}
              inputMode='numeric'
              onChange={(event) =>
                props.onDirectUserIdChange(event.target.value)
              }
              placeholder={t('Peer user ID')}
            />
            <Button
              type='button'
              onClick={props.onCreateDirect}
              disabled={props.creatingDirect}
            >
              {props.creatingDirect ? (
                <Loader2 className='h-4 w-4 animate-spin' />
              ) : (
                <Plus className='h-4 w-4' />
              )}
            </Button>
          </div>
        </div>
        <div className='bg-muted/20 rounded-2xl border p-3'>
          <div className='text-muted-foreground mb-2 flex items-center gap-2 text-xs font-medium tracking-wide uppercase'>
            <Users className='h-3.5 w-3.5' />
            {t('Group workspace')}
          </div>
          <div className='space-y-2'>
            <Input
              value={props.groupTitle}
              onChange={(event) => props.onGroupTitleChange(event.target.value)}
              placeholder={t('Group name')}
            />
            <div className='flex gap-2'>
              <Input
                value={props.groupMemberIds}
                onChange={(event) =>
                  props.onGroupMemberIdsChange(event.target.value)
                }
                placeholder={t('Member IDs, separated by commas')}
              />
              <Button
                type='button'
                variant='secondary'
                onClick={props.onCreateGroup}
                disabled={props.creatingGroup}
              >
                {props.creatingGroup ? (
                  <Loader2 className='h-4 w-4 animate-spin' />
                ) : (
                  <Users className='h-4 w-4' />
                )}
              </Button>
            </div>
          </div>
        </div>
      </CardContent>
    </Card>
  )
}
