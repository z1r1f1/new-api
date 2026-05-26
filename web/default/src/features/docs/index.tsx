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
import { useState, type ElementType } from 'react'
import {
  ArrowRight,
  Check,
  ClipboardList,
  Copy,
  KeyRound,
  Route,
  ShieldCheck,
  Sparkles,
  TerminalSquare,
  WalletCards,
  Zap,
} from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Markdown } from '@/components/ui/markdown'
import { PublicLayout } from '@/components/layout'
import {
  USER_GUIDE_INTRO,
  USER_GUIDE_SECTIONS,
  USER_GUIDE_TITLE,
} from './user-guide-content'

const connectionFields = [
  {
    label: 'Base URL',
    value: 'https://pazom.xyz/v1',
    helper: '第三方客户端的 API 地址填写这里',
  },
  {
    label: 'Authorization',
    value: 'Bearer sk-你的密钥',
    helper: '在 API 密钥页面创建后替换 sk-你的密钥',
  },
] as const

const endpointChips = [
  '/v1/chat/completions',
  '/v1/responses',
  '/v1/images/generations',
] as const

const launchSteps = [
  {
    title: '创建密钥',
    description: '复制 API Key，并按需设置模型限制或 IP 白名单。',
    icon: KeyRound,
  },
  {
    title: '填写客户端',
    description: 'Base URL 使用 pazom.xyz/v1，鉴权方式使用 Bearer。',
    icon: TerminalSquare,
  },
  {
    title: '先跑测试',
    description: '在游乐场或客户端发送短消息，确认模型和余额可用。',
    icon: Zap,
  },
  {
    title: '看日志排错',
    description: '遇到异常时复制请求 ID，再结合状态码定位原因。',
    icon: ClipboardList,
  },
] as const

const supportPanels = [
  {
    title: '余额与签到',
    description: '充值、兑换、每日奖励和实际扣费都以日志与钱包展示为准。',
    icon: WalletCards,
  },
  {
    title: '模型可用性',
    description: '模型名、分组、余额和密钥限制都会影响最终可用范围。',
    icon: Sparkles,
  },
  {
    title: '安全边界',
    description: '密钥不要外泄；发现异常消耗时立即删除旧 Key。',
    icon: ShieldCheck,
  },
] as const

const docsMarkdownClassName = 'docs-markdown prose-neutral dark:prose-invert'

type CopyFieldProps = {
  label: string
  value: string
  helper: string
}

function CopyField(props: CopyFieldProps) {
  const [copied, setCopied] = useState(false)
  const Icon = copied ? Check : Copy

  async function handleCopy() {
    try {
      await navigator.clipboard.writeText(props.value)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1600)
    } catch {
      setCopied(false)
    }
  }

  return (
    <div className='group rounded-2xl border border-white/10 bg-white/[0.04] p-4 shadow-[inset_0_1px_0_rgba(255,255,255,0.08)]'>
      <div className='flex items-center justify-between gap-3'>
        <p className='text-xs font-medium tracking-[0.22em] text-amber-200/70 uppercase'>
          {props.label}
        </p>
        <button
          type='button'
          onClick={() => void handleCopy()}
          className='rounded-full border border-white/10 bg-white/[0.06] p-2 text-white/70 transition hover:bg-white/10 hover:text-white focus-visible:ring-2 focus-visible:ring-amber-300 focus-visible:outline-none'
          aria-label={`复制 ${props.label}`}
        >
          <Icon className='h-3.5 w-3.5' aria-hidden='true' />
        </button>
      </div>
      <p className='mt-3 font-mono text-sm font-semibold break-all text-white sm:text-base'>
        {props.value}
      </p>
      <p className='mt-2 text-xs leading-5 text-white/55'>{props.helper}</p>
    </div>
  )
}

type LaunchStepProps = {
  stepNumber: string
  title: string
  description: string
  icon: ElementType
}

function LaunchStep(props: LaunchStepProps) {
  const Icon = props.icon

  return (
    <div className='group bg-card relative overflow-hidden rounded-[1.6rem] border p-5 shadow-sm transition duration-300 hover:-translate-y-1 hover:shadow-xl'>
      <div className='via-primary absolute inset-x-0 top-0 h-1 bg-gradient-to-r from-amber-400 to-cyan-400 opacity-70' />
      <div className='flex items-start justify-between gap-4'>
        <div className='bg-primary/10 text-primary rounded-2xl p-3'>
          <Icon className='h-5 w-5' aria-hidden='true' />
        </div>
        <span className='text-muted-foreground/20 group-hover:text-primary/30 font-mono text-3xl font-black transition'>
          {props.stepNumber}
        </span>
      </div>
      <h2 className='mt-5 text-lg font-semibold tracking-tight'>
        {props.title}
      </h2>
      <p className='text-muted-foreground mt-2 text-sm leading-6'>
        {props.description}
      </p>
    </div>
  )
}

type SupportPanelProps = {
  title: string
  description: string
  icon: ElementType
}

function SupportPanel(props: SupportPanelProps) {
  const Icon = props.icon

  return (
    <div className='bg-muted/25 rounded-3xl border p-5'>
      <div className='flex items-center gap-3'>
        <div className='bg-background text-primary rounded-2xl p-2 shadow-sm'>
          <Icon className='h-5 w-5' aria-hidden='true' />
        </div>
        <h2 className='font-semibold'>{props.title}</h2>
      </div>
      <p className='text-muted-foreground mt-3 text-sm leading-6'>
        {props.description}
      </p>
    </div>
  )
}

type GuideSectionProps = {
  sectionNumber: string
  section: (typeof USER_GUIDE_SECTIONS)[number]
}

function GuideSection(props: GuideSectionProps) {
  return (
    <section
      id={props.section.id}
      className='bg-card/90 scroll-mt-24 rounded-[2rem] border p-6 shadow-sm sm:p-8 lg:p-10'
    >
      <div className='mb-8 flex flex-col gap-3 border-b pb-6 sm:flex-row sm:items-start sm:justify-between'>
        <div>
          <div className='flex items-start gap-3'>
            <span className='bg-primary/10 text-primary mt-1 rounded-full px-3 py-1 font-mono text-xs font-bold'>
              {props.sectionNumber}
            </span>
            <h2 className='text-2xl font-black tracking-[-0.035em] text-balance sm:text-3xl'>
              {props.section.title}
            </h2>
          </div>
          <p className='text-muted-foreground mt-3 max-w-2xl text-base leading-7'>
            {props.section.summary}
          </p>
        </div>
      </div>
      <Markdown className={docsMarkdownClassName}>
        {props.section.content}
      </Markdown>
    </section>
  )
}

export function Docs() {
  return (
    <PublicLayout>
      <main className='bg-background relative min-h-dvh overflow-hidden'>
        <div className='pointer-events-none absolute inset-0 -z-10'>
          <div className='absolute top-0 left-1/2 h-[34rem] w-[34rem] -translate-x-1/2 rounded-full bg-amber-400/15 blur-3xl' />
          <div className='absolute top-56 -right-32 h-[28rem] w-[28rem] rounded-full bg-cyan-400/10 blur-3xl' />
          <div className='bg-primary/10 absolute top-[42rem] -left-40 h-[28rem] w-[28rem] rounded-full blur-3xl' />
        </div>

        <section className='mx-auto grid w-full max-w-7xl gap-8 px-4 pt-10 pb-8 sm:px-6 lg:grid-cols-[minmax(0,1fr)_420px] lg:px-8 lg:pt-16'>
          <div className='flex flex-col justify-center gap-8'>
            <div className='space-y-6'>
              <Badge
                className='w-fit rounded-full px-4 py-1.5'
                variant='secondary'
              >
                Pazom Field Guide
              </Badge>
              <div className='space-y-5'>
                <h1 className='max-w-4xl text-4xl font-black tracking-[-0.045em] text-balance sm:text-6xl lg:text-7xl'>
                  {USER_GUIDE_TITLE}
                </h1>
                <p className='text-muted-foreground max-w-2xl text-lg leading-8 sm:text-xl'>
                  {USER_GUIDE_INTRO}
                </p>
              </div>
            </div>

            <div className='flex flex-wrap gap-3'>
              <a
                href='#clients'
                className='bg-primary text-primary-foreground shadow-primary/20 focus-visible:ring-ring inline-flex items-center gap-2 rounded-full px-5 py-3 text-sm font-semibold shadow-lg transition hover:-translate-y-0.5 hover:shadow-xl focus-visible:ring-2 focus-visible:outline-none'
              >
                配置客户端
                <ArrowRight className='h-4 w-4' aria-hidden='true' />
              </a>
              <a
                href='#faq'
                className='bg-background/80 hover:bg-muted focus-visible:ring-ring inline-flex items-center gap-2 rounded-full border px-5 py-3 text-sm font-semibold shadow-sm backdrop-blur transition hover:-translate-y-0.5 focus-visible:ring-2 focus-visible:outline-none'
              >
                查看排错清单
              </a>
            </div>
          </div>

          <div className='relative'>
            <div className='via-primary/10 absolute -inset-4 rounded-[2.3rem] bg-gradient-to-br from-amber-300/25 to-cyan-300/20 blur-2xl' />
            <div className='relative overflow-hidden rounded-[2rem] border border-white/10 bg-slate-950 p-5 text-white shadow-2xl'>
              <div className='absolute inset-0 bg-[linear-gradient(120deg,rgba(255,255,255,0.08),transparent_34%,rgba(255,255,255,0.04))]' />
              <div className='relative space-y-5'>
                <div className='flex items-center justify-between gap-3'>
                  <div className='flex items-center gap-2'>
                    <span className='h-3 w-3 rounded-full bg-red-400' />
                    <span className='h-3 w-3 rounded-full bg-amber-300' />
                    <span className='h-3 w-3 rounded-full bg-emerald-400' />
                  </div>
                  <div className='flex items-center gap-2 rounded-full border border-white/10 bg-white/[0.04] px-3 py-1 text-xs text-white/60'>
                    <Route className='h-3.5 w-3.5' aria-hidden='true' />
                    OpenAI compatible
                  </div>
                </div>

                <div>
                  <p className='text-xs font-medium tracking-[0.28em] text-white/45 uppercase'>
                    Connection Kit
                  </p>
                  <h2 className='mt-2 text-2xl font-bold tracking-tight'>
                    三项配置，完成接入
                  </h2>
                </div>

                <div className='space-y-3'>
                  {connectionFields.map((item) => (
                    <CopyField
                      key={item.label}
                      label={item.label}
                      value={item.value}
                      helper={item.helper}
                    />
                  ))}
                </div>

                <div className='rounded-2xl border border-white/10 bg-white/[0.04] p-4'>
                  <p className='mb-3 text-xs font-medium tracking-[0.22em] text-cyan-100/70 uppercase'>
                    常用端点
                  </p>
                  <div className='flex flex-wrap gap-2'>
                    {endpointChips.map((endpoint) => (
                      <span
                        key={endpoint}
                        className='rounded-full bg-white/[0.07] px-3 py-1.5 font-mono text-xs text-white/75'
                      >
                        {endpoint}
                      </span>
                    ))}
                  </div>
                </div>
              </div>
            </div>
          </div>
        </section>

        <section className='mx-auto grid w-full max-w-7xl gap-4 px-4 py-6 sm:px-6 md:grid-cols-2 lg:grid-cols-4 lg:px-8'>
          {launchSteps.map((item, index) => (
            <LaunchStep
              key={item.title}
              stepNumber={String(index + 1).padStart(2, '0')}
              title={item.title}
              description={item.description}
              icon={item.icon}
            />
          ))}
        </section>

        <section className='mx-auto grid w-full max-w-7xl justify-center gap-8 px-4 py-10 sm:px-6 lg:grid-cols-[minmax(0,860px)_320px] lg:px-8'>
          <div className='min-w-0 space-y-8'>
            <div className='grid gap-4 md:grid-cols-3'>
              {supportPanels.map((item) => (
                <SupportPanel
                  key={item.title}
                  title={item.title}
                  description={item.description}
                  icon={item.icon}
                />
              ))}
            </div>

            <div className='space-y-8'>
              {USER_GUIDE_SECTIONS.map((section, index) => (
                <GuideSection
                  key={section.id}
                  sectionNumber={String(index + 1).padStart(2, '0')}
                  section={section}
                />
              ))}
            </div>
          </div>

          <aside className='order-first lg:sticky lg:top-20 lg:order-last lg:self-start'>
            <div className='bg-background/85 rounded-[2rem] border p-4 shadow-xl shadow-black/5 backdrop-blur'>
              <div className='bg-muted/40 rounded-[1.45rem] p-4'>
                <p className='text-muted-foreground text-xs font-semibold tracking-[0.22em] uppercase'>
                  Contents
                </p>
                <h2 className='mt-2 text-lg font-bold'>文档目录</h2>
              </div>
              <nav
                aria-label='Pazom 文档目录'
                className='mt-3 max-h-[70vh] space-y-1 overflow-auto pr-1'
              >
                {USER_GUIDE_SECTIONS.map((section, index) => (
                  <a
                    key={section.id}
                    href={`#${section.id}`}
                    className='group hover:bg-muted focus-visible:ring-ring flex gap-3 rounded-2xl px-3 py-3 text-sm transition focus-visible:ring-2 focus-visible:outline-none'
                  >
                    <span className='text-muted-foreground group-hover:text-primary mt-0.5 font-mono text-xs'>
                      {String(index + 1).padStart(2, '0')}
                    </span>
                    <span>
                      <span className='block leading-5 font-medium'>
                        {section.title}
                      </span>
                      <span className='text-muted-foreground mt-1 block text-xs leading-5'>
                        {section.summary}
                      </span>
                    </span>
                  </a>
                ))}
              </nav>
            </div>
          </aside>
        </section>
      </main>
    </PublicLayout>
  )
}
