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
import { useEffect, useMemo } from 'react'
import * as z from 'zod'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

const ipBlacklistSchema = z.object({
  ip_blacklist_setting: z.object({
    enabled: z.boolean(),
    list: z.string(),
  }),
})

type IPBlacklistFormValues = z.output<typeof ipBlacklistSchema>
type IPBlacklistFormInput = z.input<typeof ipBlacklistSchema>

type IPBlacklistSectionProps = {
  defaultValues: {
    'ip_blacklist_setting.enabled': boolean
    'ip_blacklist_setting.list': string
  }
}

type NormalizedIPBlacklistValues = IPBlacklistSectionProps['defaultValues']

const buildFormDefaults = (
  defaults: IPBlacklistSectionProps['defaultValues']
): IPBlacklistFormInput => ({
  ip_blacklist_setting: {
    enabled: defaults['ip_blacklist_setting.enabled'],
    list: defaults['ip_blacklist_setting.list'],
  },
})

const normalizeDefaults = (
  defaults: IPBlacklistSectionProps['defaultValues']
): NormalizedIPBlacklistValues => ({
  'ip_blacklist_setting.enabled': defaults['ip_blacklist_setting.enabled'],
  'ip_blacklist_setting.list': defaults['ip_blacklist_setting.list'],
})

const normalizeFormValues = (
  values: IPBlacklistFormValues
): NormalizedIPBlacklistValues => ({
  'ip_blacklist_setting.enabled': values.ip_blacklist_setting.enabled,
  'ip_blacklist_setting.list': values.ip_blacklist_setting.list.trim(),
})

export function IPBlacklistSection(props: IPBlacklistSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()

  const formDefaults = useMemo(
    () => buildFormDefaults(props.defaultValues),
    [props.defaultValues]
  )

  const form = useForm<IPBlacklistFormInput, unknown, IPBlacklistFormValues>({
    resolver: zodResolver(ipBlacklistSchema),
    defaultValues: formDefaults,
  })

  useEffect(() => {
    form.reset(buildFormDefaults(props.defaultValues))
  }, [props.defaultValues, form])

  const onSubmit = async (data: IPBlacklistFormValues) => {
    const normalized = normalizeFormValues(data)
    const baseline = normalizeDefaults(props.defaultValues)
    const keys = Object.keys(normalized) as Array<
      keyof NormalizedIPBlacklistValues
    >
    const updates = keys.filter((key) => normalized[key] !== baseline[key])

    if (updates.length === 0) {
      toast.info(t('No changes to save'))
      return
    }

    for (const key of updates) {
      await updateOption.mutateAsync({
        key,
        value: normalized[key],
      })
    }

    form.reset(buildFormDefaults(normalized))
  }

  return (
    <SettingsSection
      title={t('IP Blacklist')}
      description={t(
        'Block requests from specified client IPs or CIDR ranges.'
      )}
    >
      <Form {...form}>
        <form onSubmit={form.handleSubmit(onSubmit)} className='space-y-6'>
          <FormField
            control={form.control}
            name='ip_blacklist_setting.enabled'
            render={({ field }) => (
              <FormItem className='flex flex-row items-center justify-between rounded-lg border p-4'>
                <div className='space-y-0.5'>
                  <FormLabel className='text-base'>
                    {t('Enable IP Blacklist')}
                  </FormLabel>
                  <FormDescription>
                    {t(
                      'Reject matching requests before authentication and routing.'
                    )}
                  </FormDescription>
                </div>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='ip_blacklist_setting.list'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Blocked IPs')}</FormLabel>
                <FormControl>
                  <Textarea
                    placeholder={t('192.168.1.1&#10;10.0.0.0/8')}
                    rows={8}
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'One IP or CIDR range per line. Commas and semicolons are also supported.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <Button type='submit' disabled={updateOption.isPending}>
            {updateOption.isPending ? t('Saving...') : t('Save IP blacklist')}
          </Button>
        </form>
      </Form>
    </SettingsSection>
  )
}
