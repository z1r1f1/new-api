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
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
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
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

const autoBanScopes = ['all', 'relay', 'custom'] as const
type AutoBanScope = (typeof autoBanScopes)[number]

const defaultAutoBanRelayPathPrefixes = [
  '/v1',
  '/v1beta',
  '/pg/chat/completions',
  '/pg/images',
  '/mj',
  '/suno',
  '/kling/v1',
  '/jimeng',
]

const normalizeAutoBanScope = (scope: string): AutoBanScope => {
  return autoBanScopes.includes(scope as AutoBanScope)
    ? (scope as AutoBanScope)
    : 'relay'
}

const splitAutoBanPathPrefixes = (raw: string): string[] => {
  const prefixes = raw
    .replaceAll(',', '\n')
    .replaceAll(';', '\n')
    .split('\n')
    .map((part) => part.trim())
    .filter(Boolean)
    .map((prefix) => (prefix.startsWith('/') ? prefix : `/${prefix}`))

  return [...new Set(prefixes)]
}

const ipBlacklistSchema = z.object({
  ip_blacklist_setting: z.object({
    enabled: z.boolean(),
    list: z.string(),
    auto_ban_enabled: z.boolean(),
    auto_ban_rpm: z.coerce.number().int().min(0).max(100000000),
    auto_ban_whitelist: z.string(),
    auto_ban_scope: z.enum(autoBanScopes),
    auto_ban_path_prefixes: z.string(),
  }),
})

type IPBlacklistFormValues = z.output<typeof ipBlacklistSchema>
type IPBlacklistFormInput = z.input<typeof ipBlacklistSchema>

type IPBlacklistSectionProps = {
  defaultValues: {
    'ip_blacklist_setting.enabled': boolean
    'ip_blacklist_setting.list': string
    'ip_blacklist_setting.auto_ban_enabled': boolean
    'ip_blacklist_setting.auto_ban_rpm': number
    'ip_blacklist_setting.auto_ban_whitelist': string
    'ip_blacklist_setting.auto_ban_scope': string
    'ip_blacklist_setting.auto_ban_path_prefixes': string
  }
}

type NormalizedIPBlacklistValues = IPBlacklistSectionProps['defaultValues']

const buildFormDefaults = (
  defaults: IPBlacklistSectionProps['defaultValues']
): IPBlacklistFormInput => ({
  ip_blacklist_setting: {
    enabled: defaults['ip_blacklist_setting.enabled'],
    list: defaults['ip_blacklist_setting.list'],
    auto_ban_enabled: defaults['ip_blacklist_setting.auto_ban_enabled'],
    auto_ban_rpm: defaults['ip_blacklist_setting.auto_ban_rpm'],
    auto_ban_whitelist: defaults['ip_blacklist_setting.auto_ban_whitelist'],
    auto_ban_scope: normalizeAutoBanScope(
      defaults['ip_blacklist_setting.auto_ban_scope']
    ),
    auto_ban_path_prefixes:
      defaults['ip_blacklist_setting.auto_ban_path_prefixes'],
  },
})

const normalizeDefaults = (
  defaults: IPBlacklistSectionProps['defaultValues']
): NormalizedIPBlacklistValues => ({
  'ip_blacklist_setting.enabled': defaults['ip_blacklist_setting.enabled'],
  'ip_blacklist_setting.list': defaults['ip_blacklist_setting.list'],
  'ip_blacklist_setting.auto_ban_enabled':
    defaults['ip_blacklist_setting.auto_ban_enabled'],
  'ip_blacklist_setting.auto_ban_rpm':
    defaults['ip_blacklist_setting.auto_ban_rpm'],
  'ip_blacklist_setting.auto_ban_whitelist':
    defaults['ip_blacklist_setting.auto_ban_whitelist'],
  'ip_blacklist_setting.auto_ban_scope': normalizeAutoBanScope(
    defaults['ip_blacklist_setting.auto_ban_scope']
  ),
  'ip_blacklist_setting.auto_ban_path_prefixes':
    defaults['ip_blacklist_setting.auto_ban_path_prefixes'],
})

const normalizeFormValues = (
  values: IPBlacklistFormValues
): NormalizedIPBlacklistValues => ({
  'ip_blacklist_setting.enabled': values.ip_blacklist_setting.enabled,
  'ip_blacklist_setting.list': values.ip_blacklist_setting.list.trim(),
  'ip_blacklist_setting.auto_ban_enabled':
    values.ip_blacklist_setting.auto_ban_enabled,
  'ip_blacklist_setting.auto_ban_rpm':
    values.ip_blacklist_setting.auto_ban_rpm,
  'ip_blacklist_setting.auto_ban_whitelist':
    values.ip_blacklist_setting.auto_ban_whitelist.trim(),
  'ip_blacklist_setting.auto_ban_scope':
    values.ip_blacklist_setting.auto_ban_scope,
  'ip_blacklist_setting.auto_ban_path_prefixes':
    values.ip_blacklist_setting.auto_ban_path_prefixes.trim(),
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
  const autoBanScope = form.watch('ip_blacklist_setting.auto_ban_scope')
  const autoBanPathPrefixes = form.watch(
    'ip_blacklist_setting.auto_ban_path_prefixes'
  )

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

  const getAutoBanScopeDescription = () => {
    switch (autoBanScope) {
      case 'all':
        return t(
          'Counts every HTTP request, including frontend pages and dashboard APIs.'
        )
      case 'custom':
        return t('Only counts paths that match the custom prefixes above.')
      default:
        return t(
          'Counts relay/model API paths such as /v1, /pg, /mj, /suno, and video generation APIs.'
        )
    }
  }

  const getAutoBanScopeDetail = () => {
    switch (autoBanScope) {
      case 'all':
        return {
          modeLabel: t('All requests'),
          summary: t(
            'Counts every HTTP request, including frontend pages and dashboard APIs.'
          ),
          prefixes: [],
          emptyText: t(
            'Every HTTP request is included; there is no path-prefix filter.'
          ),
        }
      case 'custom': {
        const prefixes = splitAutoBanPathPrefixes(autoBanPathPrefixes)
        return {
          modeLabel: t('Custom path prefixes'),
          summary: t('Only counts paths that match the custom prefixes above.'),
          prefixes,
          emptyText: t(
            'No custom path prefixes configured. Custom scope will not count any requests.'
          ),
        }
      }
      default:
        return {
          modeLabel: t('Relay/model API requests'),
          summary: t(
            'Counts relay/model API paths such as /v1, /pg, /mj, /suno, and video generation APIs.'
          ),
          prefixes: defaultAutoBanRelayPathPrefixes,
          emptyText: '',
        }
    }
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

          <FormField
            control={form.control}
            name='ip_blacklist_setting.auto_ban_enabled'
            render={({ field }) => (
              <FormItem className='flex flex-row items-center justify-between rounded-lg border p-4'>
                <div className='space-y-0.5'>
                  <FormLabel className='text-base'>
                    {t('Enable Automatic IP Ban')}
                  </FormLabel>
                  <FormDescription>
                    {t(
                      'Automatically add high-RPM client IPs to the blacklist and reject them.'
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

          <div className='grid gap-4 md:grid-cols-2'>
            <FormField
              control={form.control}
              name='ip_blacklist_setting.auto_ban_rpm'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Auto-ban RPM threshold')}</FormLabel>
                  <FormControl>
                    <div className='flex items-center gap-2'>
                      <Input
                        type='number'
                        min={0}
                        max={100000000}
                        step={1}
                        name={field.name}
                        ref={field.ref}
                        onBlur={field.onBlur}
                        value={
                          typeof field.value === 'number' ? field.value : 0
                        }
                        onChange={(e) =>
                          field.onChange(parseInt(e.target.value) || 0)
                        }
                      />
                      <span className='text-muted-foreground text-sm whitespace-nowrap'>
                        {t('requests/min')}
                      </span>
                    </div>
                  </FormControl>
                  <FormDescription>
                    {t('0 disables automatic banning.')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='ip_blacklist_setting.auto_ban_whitelist'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Auto-ban Whitelist')}</FormLabel>
                  <FormControl>
                    <Textarea
                      placeholder={t('192.168.1.1&#10;10.0.0.0/8')}
                      rows={5}
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'IPs or CIDR ranges here will never be automatically banned. Manual blacklist entries still apply.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='ip_blacklist_setting.auto_ban_scope'
              render={({ field }) => (
                <FormItem>
                  <div className='flex items-center justify-between gap-2'>
                    <FormLabel>{t('Auto-ban Counting Scope')}</FormLabel>
                    <Dialog>
                      <DialogTrigger
                        render={
                          <Button type='button' variant='outline' size='sm' />
                        }
                      >
                        {t('View details')}
                      </DialogTrigger>
                      <DialogContent className='sm:max-w-lg'>
                        <DialogHeader>
                          <DialogTitle>
                            {t('Detailed counting scope')}
                          </DialogTitle>
                          <DialogDescription>
                            {t(
                              'Review which requests are counted before an IP reaches the automatic ban RPM threshold.'
                            )}
                          </DialogDescription>
                        </DialogHeader>

                        <AutoBanScopeDetailContent
                          detail={getAutoBanScopeDetail()}
                        />

                        <DialogFooter>
                          <DialogClose
                            render={<Button type='button' variant='outline' />}
                          >
                            {t('Close')}
                          </DialogClose>
                        </DialogFooter>
                      </DialogContent>
                    </Dialog>
                  </div>
                  <FormControl>
                    <Select
                      items={[
                        {
                          value: 'relay',
                          label: t('Relay/model API requests'),
                        },
                        { value: 'all', label: t('All requests') },
                        {
                          value: 'custom',
                          label: t('Custom path prefixes'),
                        },
                      ]}
                      value={field.value}
                      onValueChange={field.onChange}
                    >
                      <SelectTrigger className='w-full'>
                        <SelectValue
                          placeholder={t('Select auto-ban counting scope')}
                        />
                      </SelectTrigger>
                      <SelectContent alignItemWithTrigger={false}>
                        <SelectGroup>
                          <SelectItem value='relay'>
                            {t('Relay/model API requests')}
                          </SelectItem>
                          <SelectItem value='all'>
                            {t('All requests')}
                          </SelectItem>
                          <SelectItem value='custom'>
                            {t('Custom path prefixes')}
                          </SelectItem>
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Choose which request paths count toward the automatic IP ban RPM threshold.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          {autoBanScope === 'custom' ? (
            <FormField
              control={form.control}
              name='ip_blacklist_setting.auto_ban_path_prefixes'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Auto-ban Path Prefixes')}</FormLabel>
                  <FormControl>
                    <Textarea
                      placeholder={'/v1&#10;/pg/chat/completions'}
                      rows={5}
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'One path prefix per line. Commas and semicolons are also supported.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          ) : null}

          <div className='rounded-lg border bg-muted/30 p-4 text-sm text-muted-foreground'>
            {getAutoBanScopeDescription()}
          </div>

          <Button type='submit' disabled={updateOption.isPending}>
            {updateOption.isPending ? t('Saving...') : t('Save configuration')}
          </Button>
        </form>
      </Form>
    </SettingsSection>
  )
}

type AutoBanScopeDetailContentProps = {
  detail: {
    modeLabel: string
    summary: string
    prefixes: string[]
    emptyText: string
  }
}

function AutoBanScopeDetailContent(props: AutoBanScopeDetailContentProps) {
  const { t } = useTranslation()

  return (
    <div className='space-y-4'>
      <div className='rounded-lg border p-3'>
        <div className='text-muted-foreground text-xs font-medium tracking-wide uppercase'>
          {t('Current scope')}
        </div>
        <div className='mt-1 font-medium'>{props.detail.modeLabel}</div>
        <p className='text-muted-foreground mt-1 text-sm'>
          {props.detail.summary}
        </p>
      </div>

      <div className='space-y-2'>
        <div className='text-sm font-medium'>{t('Matched path prefixes')}</div>
        {props.detail.prefixes.length > 0 ? (
          <ul className='grid gap-2 rounded-lg border bg-muted/30 p-3 text-sm sm:grid-cols-2'>
            {props.detail.prefixes.map((prefix) => (
              <li key={prefix} className='font-mono text-xs'>
                {prefix}
              </li>
            ))}
          </ul>
        ) : (
          <p className='text-muted-foreground rounded-lg border bg-muted/30 p-3 text-sm'>
            {props.detail.emptyText}
          </p>
        )}
      </div>

      <p className='text-muted-foreground rounded-lg bg-muted/40 p-3 text-sm'>
        {t(
          'Manual blacklist entries are still enforced globally and are not limited by this counting scope.'
        )}
      </p>
    </div>
  )
}
