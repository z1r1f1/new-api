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
  BookOpen,
  CheckCircle2,
  ClipboardList,
  KeyRound,
  MessageSquareText,
  ShieldCheck,
  Sparkles,
  WalletCards,
} from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'
import { Markdown } from '@/components/ui/markdown'
import { PublicLayout } from '@/components/layout'
import {
  USER_GUIDE_INTRO,
  USER_GUIDE_SECTIONS,
  USER_GUIDE_TITLE,
} from './user-guide-content'

const heroStats = [
  {
    label: 'Base URL',
    value: 'https://pazom.xyz/v1',
    description: 'OpenAI 兼容客户端统一填写这个地址。',
  },
  {
    label: '鉴权方式',
    value: 'Bearer API Key',
    description: '在 API 密钥页面创建并复制自己的密钥。',
  },
  {
    label: '建议流程',
    value: '先测试再接入',
    description: '先用游乐场验证模型，再配置到常用客户端。',
  },
] as const

const quickCards = [
  {
    title: '创建密钥',
    description: '进入 API 密钥页面创建 Key，按需设置模型限制或 IP 白名单。',
    icon: KeyRound,
  },
  {
    title: '配置客户端',
    description:
      '在 Cherry Studio、Chatbox 等客户端中填写 Base URL 和 API Key。',
    icon: Sparkles,
  },
  {
    title: '测试模型',
    description:
      '通过游乐场或客户端发送一次简单请求，确认模型、分组和余额可用。',
    icon: MessageSquareText,
  },
  {
    title: '查看日志',
    description: '遇到错误时复制请求 ID，结合状态码、路径和首字时间快速定位。',
    icon: ClipboardList,
  },
] as const

const supportCards = [
  {
    title: '余额与签到',
    description: '了解余额、额度、签到奖励与实际扣费记录。',
    icon: WalletCards,
  },
  {
    title: '安全建议',
    description: '保护 API Key，避免公开分享、提交仓库或暴露在截图中。',
    icon: ShieldCheck,
  },
  {
    title: '排错清单',
    description: '按 401、403、408、429、500、流式和图片问题逐项排查。',
    icon: CheckCircle2,
  },
] as const

export function Docs() {
  return (
    <PublicLayout>
      <main className='bg-background min-h-dvh'>
        <section className='relative overflow-hidden border-b'>
          <div className='from-primary/10 via-background to-background absolute inset-0 bg-gradient-to-br' />
          <div className='relative mx-auto grid w-full max-w-6xl gap-8 px-4 py-10 sm:px-6 lg:grid-cols-[minmax(0,1fr)_360px] lg:px-8 lg:py-14'>
            <div className='space-y-6'>
              <div className='space-y-4'>
                <Badge variant='secondary' className='w-fit'>
                  Pazom Docs
                </Badge>
                <div className='space-y-3'>
                  <h1 className='text-3xl font-bold tracking-tight sm:text-5xl'>
                    {USER_GUIDE_TITLE}
                  </h1>
                  <p className='text-muted-foreground max-w-3xl text-base leading-7 sm:text-lg'>
                    {USER_GUIDE_INTRO}
                  </p>
                </div>
              </div>

              <div className='grid gap-3 sm:grid-cols-3'>
                {heroStats.map((item) => (
                  <div
                    key={item.label}
                    className='bg-card/80 rounded-2xl border p-4 shadow-sm backdrop-blur'
                  >
                    <p className='text-muted-foreground text-xs font-medium tracking-wide uppercase'>
                      {item.label}
                    </p>
                    <p className='mt-2 font-mono text-sm font-semibold break-all'>
                      {item.value}
                    </p>
                    <p className='text-muted-foreground mt-2 text-sm leading-6'>
                      {item.description}
                    </p>
                  </div>
                ))}
              </div>
            </div>

            <Card className='bg-card/85 border-primary/15 self-end shadow-lg backdrop-blur'>
              <CardContent className='space-y-4'>
                <div className='flex items-center gap-3'>
                  <div className='bg-primary/10 text-primary rounded-xl p-2.5'>
                    <BookOpen className='h-5 w-5' aria-hidden='true' />
                  </div>
                  <div>
                    <h2 className='font-semibold'>推荐阅读顺序</h2>
                    <p className='text-muted-foreground text-sm'>
                      按步骤完成配置，再查看排错和安全建议。
                    </p>
                  </div>
                </div>
                <ol className='space-y-3 text-sm'>
                  {quickCards.slice(0, 3).map((item, index) => (
                    <li key={item.title} className='flex gap-3'>
                      <span className='bg-primary text-primary-foreground flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-xs font-semibold'>
                        {index + 1}
                      </span>
                      <span className='leading-6'>{item.title}</span>
                    </li>
                  ))}
                </ol>
              </CardContent>
            </Card>
          </div>
        </section>

        <section className='mx-auto grid w-full max-w-6xl gap-4 px-4 py-8 sm:px-6 lg:grid-cols-4 lg:px-8'>
          {quickCards.map((item) => {
            const Icon = item.icon
            return (
              <Card key={item.title} className='bg-card/70'>
                <CardContent className='space-y-3'>
                  <div className='bg-primary/10 text-primary inline-flex rounded-xl p-2'>
                    <Icon className='h-5 w-5' aria-hidden='true' />
                  </div>
                  <div className='space-y-1'>
                    <h2 className='font-semibold'>{item.title}</h2>
                    <p className='text-muted-foreground text-sm leading-6'>
                      {item.description}
                    </p>
                  </div>
                </CardContent>
              </Card>
            )
          })}
        </section>

        <section className='mx-auto grid w-full max-w-6xl gap-6 px-4 pb-12 sm:px-6 lg:grid-cols-[280px_minmax(0,1fr)] lg:px-8'>
          <aside className='lg:sticky lg:top-20 lg:self-start'>
            <Card className='bg-card/80'>
              <CardContent className='space-y-5'>
                <div>
                  <h2 className='text-sm font-semibold'>文档目录</h2>
                  <p className='text-muted-foreground mt-1 text-xs leading-5'>
                    点击条目可跳转到对应章节。
                  </p>
                </div>
                <nav aria-label='Pazom 文档目录' className='space-y-1'>
                  {USER_GUIDE_SECTIONS.map((section) => (
                    <a
                      key={section.id}
                      href={`#${section.id}`}
                      className='hover:bg-muted focus-visible:ring-ring block rounded-lg px-3 py-2 text-sm transition-colors outline-none focus-visible:ring-2'
                    >
                      <span className='font-medium'>{section.title}</span>
                      <span className='text-muted-foreground mt-0.5 block text-xs leading-5'>
                        {section.summary}
                      </span>
                    </a>
                  ))}
                </nav>
              </CardContent>
            </Card>
          </aside>

          <div className='space-y-6'>
            <div className='grid gap-4 md:grid-cols-3'>
              {supportCards.map((item) => {
                const Icon = item.icon
                return (
                  <Card key={item.title} size='sm' className='bg-muted/25'>
                    <CardContent className='space-y-2'>
                      <Icon
                        className='text-primary h-5 w-5'
                        aria-hidden='true'
                      />
                      <h2 className='font-semibold'>{item.title}</h2>
                      <p className='text-muted-foreground text-sm leading-6'>
                        {item.description}
                      </p>
                    </CardContent>
                  </Card>
                )
              })}
            </div>

            <article className='bg-card rounded-2xl border px-4 py-6 shadow-sm sm:px-8 sm:py-8'>
              <div className='space-y-10'>
                {USER_GUIDE_SECTIONS.map((section) => (
                  <section
                    key={section.id}
                    id={section.id}
                    className='scroll-mt-24 border-b pb-8 last:border-b-0 last:pb-0'
                  >
                    <Markdown className='prose-neutral dark:prose-invert'>
                      {`## ${section.title}\n\n${section.content}`}
                    </Markdown>
                  </section>
                ))}
              </div>
            </article>
          </div>
        </section>
      </main>
    </PublicLayout>
  )
}
