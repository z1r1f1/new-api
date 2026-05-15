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
import { useCallback, useRef, useState } from 'react'
import type { FileUIPart } from 'ai'
import {
  PaperclipIcon,
  FileIcon,
  ImageIcon,
  ScreenShareIcon,
  CameraIcon,
  GlobeIcon,
  SendIcon,
  SquareIcon,
  BarChartIcon,
  BoxIcon,
  NotepadTextIcon,
  CodeSquareIcon,
  GraduationCapIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  PromptInput,
  PromptInputAttachment,
  PromptInputAttachments,
  PromptInputButton,
  PromptInputFooter,
  PromptInputHeader,
  PromptInputTextarea,
  PromptInputTools,
  usePromptInputAttachments,
  type PromptInputMessage,
} from '@/components/ai-elements/prompt-input'
import { Suggestion, Suggestions } from '@/components/ai-elements/suggestion'
import { ModelGroupSelector } from '@/components/model-group-selector'
import type { ModelOption, GroupOption } from '../types'

interface PlaygroundInputProps {
  onSubmit: (text: string) => void
  onStop?: () => void
  disabled?: boolean
  isGenerating?: boolean
  models: ModelOption[]
  modelValue: string
  onModelChange: (value: string) => void
  isModelLoading?: boolean
  groups: GroupOption[]
  groupValue: string
  onGroupChange: (value: string) => void
  showModelControls?: boolean
  searchEnabled?: boolean
  onSearchEnabledChange?: (value: boolean) => void
}

const suggestions = [
  { icon: BarChartIcon, text: 'Analyze data', color: '#76d0eb' },
  { icon: BoxIcon, text: 'Surprise me', color: '#76d0eb' },
  { icon: NotepadTextIcon, text: 'Summarize text', color: '#ea8444' },
  { icon: CodeSquareIcon, text: 'Code', color: '#6c71ff' },
  { icon: GraduationCapIcon, text: 'Get advice', color: '#76d0eb' },
  { icon: null, text: 'More' },
]

const maxTextAttachmentChars = 200_000

function isImageAttachment(file: FileUIPart): boolean {
  return Boolean(file.url && file.mediaType?.startsWith('image/'))
}

function isTextAttachment(file: FileUIPart): boolean {
  const mediaType = String(file.mediaType || '').toLowerCase()
  const filename = String(file.filename || '').toLowerCase()

  return (
    mediaType.startsWith('text/') ||
    mediaType.includes('json') ||
    mediaType.includes('xml') ||
    filename.endsWith('.md') ||
    filename.endsWith('.csv') ||
    filename.endsWith('.log') ||
    filename.endsWith('.txt')
  )
}

function decodeDataUrlText(url: string): string {
  const commaIndex = url.indexOf(',')
  if (!url.startsWith('data:') || commaIndex < 0) return ''

  const metadata = url.slice(0, commaIndex)
  const payload = url.slice(commaIndex + 1)
  if (!metadata.includes(';base64')) {
    return decodeURIComponent(payload)
  }

  const binary = globalThis.atob(payload)
  const bytes = Uint8Array.from(binary, (char) => char.charCodeAt(0))
  return new TextDecoder().decode(bytes)
}

function buildAttachmentContent(text: string, files: FileUIPart[]): string {
  const contentParts = [text.trim()].filter(Boolean)

  files.forEach((file, index) => {
    const filename = file.filename || `attachment-${index + 1}`
    if (isImageAttachment(file)) {
      contentParts.push(`![${filename}](${file.url})`)
      return
    }

    if (file.url && isTextAttachment(file)) {
      const decoded = decodeDataUrlText(file.url).slice(
        0,
        maxTextAttachmentChars
      )
      const suffix =
        decoded.length >= maxTextAttachmentChars
          ? '\n\n[Attachment truncated]'
          : ''
      contentParts.push(
        `Attached file: ${filename}\n\n\`\`\`\n${decoded}${suffix}\n\`\`\``
      )
      return
    }

    contentParts.push(`Attached file: ${filename}`)
  })

  return contentParts.join('\n\n')
}

function canvasToPngFile(canvas: HTMLCanvasElement, filename: string) {
  return new Promise<File>((resolve, reject) => {
    canvas.toBlob((blob) => {
      if (!blob) {
        reject(new Error('Canvas capture failed'))
        return
      }
      resolve(new File([blob], filename, { type: 'image/png' }))
    }, 'image/png')
  })
}

async function captureMediaStreamFrame(
  stream: MediaStream,
  filename: string
): Promise<File> {
  const video = document.createElement('video')
  video.muted = true
  video.playsInline = true
  video.srcObject = stream

  try {
    await video.play()
    if (!video.videoWidth || !video.videoHeight) {
      await new Promise<void>((resolve) => {
        video.onloadedmetadata = () => resolve()
      })
    }

    const canvas = document.createElement('canvas')
    canvas.width = video.videoWidth || 1280
    canvas.height = video.videoHeight || 720
    const context = canvas.getContext('2d')
    if (!context) {
      throw new Error('Canvas is not available')
    }
    context.drawImage(video, 0, 0, canvas.width, canvas.height)
    return await canvasToPngFile(canvas, filename)
  } finally {
    stream.getTracks().forEach((track) => track.stop())
    video.srcObject = null
  }
}

function HiddenAttachmentInputs() {
  const attachments = usePromptInputAttachments()
  const fileInputRef = useRef<HTMLInputElement | null>(null)
  const imageInputRef = useRef<HTMLInputElement | null>(null)

  const addSelectedFiles = useCallback(
    (files: FileList | null) => {
      if (files?.length) {
        attachments.add(files)
      }
    },
    [attachments]
  )

  return (
    <>
      <input
        ref={fileInputRef}
        className='hidden'
        multiple
        type='file'
        onChange={(event) => {
          addSelectedFiles(event.target.files)
          event.target.value = ''
        }}
      />
      <input
        ref={imageInputRef}
        accept='image/*'
        className='hidden'
        multiple
        type='file'
        onChange={(event) => {
          addSelectedFiles(event.target.files)
          event.target.value = ''
        }}
      />
      <AttachmentMenuItems
        onUploadFile={() => fileInputRef.current?.click()}
        onUploadPhoto={() => imageInputRef.current?.click()}
      />
    </>
  )
}

function AttachmentMenuItems(props: {
  onUploadFile: () => void
  onUploadPhoto: () => void
}) {
  const { t } = useTranslation()
  const attachments = usePromptInputAttachments()

  const captureScreenshot = async () => {
    const captureDisplayMedia = navigator.mediaDevices?.getDisplayMedia
    if (!captureDisplayMedia) {
      toast.error(t('Screen capture is not supported by this browser.'))
      return
    }

    try {
      const stream = await captureDisplayMedia.call(navigator.mediaDevices, {
        video: true,
      })
      const file = await captureMediaStreamFrame(stream, 'screenshot.png')
      attachments.add([file])
      toast.success(t('Screenshot attached'))
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : t('Failed to capture screenshot')
      )
    }
  }

  const capturePhoto = async () => {
    if (!navigator.mediaDevices?.getUserMedia) {
      toast.error(t('Camera capture is not supported by this browser.'))
      return
    }

    try {
      const stream = await navigator.mediaDevices.getUserMedia({ video: true })
      const file = await captureMediaStreamFrame(stream, 'camera-photo.png')
      attachments.add([file])
      toast.success(t('Photo attached'))
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Failed to take photo')
      )
    }
  }

  return (
    <>
      <DropdownMenuItem onClick={props.onUploadFile}>
        <FileIcon className='mr-2' size={16} />
        {t('Upload file')}
      </DropdownMenuItem>
      <DropdownMenuItem onClick={props.onUploadPhoto}>
        <ImageIcon className='mr-2' size={16} />
        {t('Upload photo')}
      </DropdownMenuItem>
      <DropdownMenuItem onClick={() => void captureScreenshot()}>
        <ScreenShareIcon className='mr-2' size={16} />
        {t('Take screenshot')}
      </DropdownMenuItem>
      <DropdownMenuItem onClick={() => void capturePhoto()}>
        <CameraIcon className='mr-2' size={16} />
        {t('Take photo')}
      </DropdownMenuItem>
    </>
  )
}

function PlaygroundSubmitButton(props: {
  disabled?: boolean
  isGenerating?: boolean
  onStop?: () => void
  text: string
}) {
  const { t } = useTranslation()
  const attachments = usePromptInputAttachments()

  if (props.isGenerating && props.onStop) {
    return (
      <PromptInputButton
        className='text-foreground font-medium'
        onClick={props.onStop}
        variant='secondary'
      >
        <SquareIcon className='fill-current' size={16} />
        <span className='hidden sm:inline'>{t('Stop')}</span>
        <span className='sr-only sm:hidden'>{t('Stop')}</span>
      </PromptInputButton>
    )
  }

  return (
    <PromptInputButton
      className='text-foreground font-medium'
      disabled={
        props.disabled || (!props.text.trim() && attachments.files.length === 0)
      }
      type='submit'
      variant='secondary'
    >
      <SendIcon size={16} />
      <span className='hidden sm:inline'>{t('Send')}</span>
      <span className='sr-only sm:hidden'>{t('Send')}</span>
    </PromptInputButton>
  )
}

function PlaygroundAttachmentPreview() {
  const attachments = usePromptInputAttachments()

  if (attachments.files.length === 0) {
    return null
  }

  return (
    <PromptInputHeader className='px-2 pt-2'>
      <PromptInputAttachments>
        {(attachment) => <PromptInputAttachment data={attachment} />}
      </PromptInputAttachments>
    </PromptInputHeader>
  )
}

export function PlaygroundInput({
  onSubmit,
  onStop,
  disabled,
  isGenerating,
  models,
  modelValue,
  onModelChange,
  isModelLoading = false,
  groups,
  groupValue,
  onGroupChange,
  showModelControls = true,
  searchEnabled = false,
  onSearchEnabledChange,
}: PlaygroundInputProps) {
  const { t } = useTranslation()
  const [text, setText] = useState('')

  const isModelSelectDisabled =
    disabled || isModelLoading || models.length === 0
  const isGroupSelectDisabled = disabled || groups.length === 0

  const handleSubmit = (message: PromptInputMessage) => {
    if (disabled) return

    let content: string
    try {
      content = buildAttachmentContent(message.text || '', message.files || [])
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Failed to read attachment')
      )
      return
    }
    if (!content.trim()) return

    onSubmit(content)
    setText('')
  }

  const handleSuggestionClick = (suggestion: string) => {
    onSubmit(suggestion)
  }

  const handleSearchToggle = () => {
    const nextValue = !searchEnabled
    onSearchEnabledChange?.(nextValue)
    toast.success(t(nextValue ? 'Web search enabled' : 'Web search disabled'))
  }

  return (
    <div className='grid shrink-0 gap-4 px-1 md:pb-4'>
      <PromptInput
        groupClassName='rounded-xl'
        globalDrop
        maxFileSize={20 * 1024 * 1024}
        multiple
        onError={(error) => toast.error(error.message)}
        onSubmit={handleSubmit}
      >
        <PlaygroundAttachmentPreview />
        <PromptInputTextarea
          autoComplete='off'
          autoCorrect='off'
          autoCapitalize='off'
          spellCheck={false}
          className='px-5 md:text-base'
          disabled={disabled}
          onChange={(event) => setText(event.target.value)}
          placeholder={t('Ask anything')}
          value={text}
        />

        <PromptInputFooter className='p-2.5'>
          <PromptInputTools>
            <DropdownMenu>
              <DropdownMenuTrigger
                render={
                  <PromptInputButton
                    className='border font-medium'
                    disabled={disabled}
                    variant='outline'
                  />
                }
              >
                <PaperclipIcon size={16} />
                <span className='hidden sm:inline'>{t('Attach')}</span>
                <span className='sr-only sm:hidden'>{t('Attach')}</span>
              </DropdownMenuTrigger>
              <DropdownMenuContent align='start'>
                <HiddenAttachmentInputs />
              </DropdownMenuContent>
            </DropdownMenu>

            <PromptInputButton
              className='border font-medium'
              disabled={disabled}
              onClick={handleSearchToggle}
              variant={searchEnabled ? 'secondary' : 'outline'}
            >
              <GlobeIcon size={16} />
              <span className='hidden sm:inline'>{t('Search')}</span>
              <span className='sr-only sm:hidden'>{t('Search')}</span>
            </PromptInputButton>
          </PromptInputTools>

          <div className='flex items-center gap-1.5 md:gap-2'>
            {showModelControls && (
              <ModelGroupSelector
                selectedModel={modelValue}
                models={models}
                onModelChange={onModelChange}
                selectedGroup={groupValue}
                groups={groups}
                onGroupChange={onGroupChange}
                disabled={isModelSelectDisabled || isGroupSelectDisabled}
              />
            )}

            <PlaygroundSubmitButton
              disabled={disabled}
              isGenerating={isGenerating}
              onStop={onStop}
              text={text}
            />
          </div>
        </PromptInputFooter>
      </PromptInput>

      <Suggestions>
        {suggestions.map(({ icon: Icon, text, color }) => (
          <Suggestion
            className={`text-xs font-normal sm:text-sm ${
              text === 'More' ? 'hidden sm:flex' : ''
            }`}
            key={text}
            onClick={() => handleSuggestionClick(text)}
            suggestion={text}
          >
            {Icon && <Icon size={16} style={{ color }} />}
            {text}
          </Suggestion>
        ))}
      </Suggestions>
    </div>
  )
}
