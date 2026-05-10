// Message types
export type MessageRole = 'user' | 'assistant' | 'system'

export type MessageStatus = 'loading' | 'streaming' | 'complete' | 'error'

export interface MessageVersion {
  id: string
  content: string
}

export interface GeneratedImage {
  url: string
  alt: string
}

export interface Message {
  key: string
  from: MessageRole
  versions: MessageVersion[]
  sources?: { href: string; title: string }[]
  reasoning?: {
    content: string
    duration: number
  }
  isReasoningStreaming?: boolean
  isReasoningComplete?: boolean
  isContentComplete?: boolean
  status?: MessageStatus
  errorCode?: string | null
}

// API payload types
export interface ChatCompletionMessage {
  role: MessageRole
  content: string | ContentPart[]
}

export interface ContentPart {
  type: 'text' | 'image_url'
  text?: string
  image_url?: {
    url: string
  }
}

export interface ChatCompletionRequest {
  model: string
  group?: string
  messages: ChatCompletionMessage[]
  stream: boolean
  temperature?: number
  top_p?: number
  max_tokens?: number
  frequency_penalty?: number
  presence_penalty?: number
  seed?: number
}

export interface ChatCompletionChunk {
  id: string
  object: string
  created: number
  model: string
  choices: Array<{
    index: number
    delta: {
      role?: MessageRole
      content?: string
      reasoning_content?: string
    }
    finish_reason: string | null
  }>
}

export interface ChatCompletionResponse {
  id: string
  object: string
  created: number
  model: string
  choices: Array<{
    index: number
    message: {
      role: MessageRole
      content: string
      reasoning_content?: string
    }
    finish_reason: string
  }>
  usage?: {
    prompt_tokens: number
    completion_tokens: number
    total_tokens: number
  }
}

export interface ImageGenerationRequest {
  model: string
  group?: string
  prompt: string
  [key: string]: unknown
}

export interface ImageGenerationDataItem {
  url?: string
  b64_json?: string
  revised_prompt?: string
}

export interface ImageGenerationResult {
  data?: ImageGenerationDataItem[]
  error?: {
    message?: string
    type?: string
    code?: string
  }
  message?: string
  [key: string]: unknown
}

export interface ImageGenerationSubmitResponse extends ImageGenerationResult {
  task_id?: string
  taskId?: string
  id?: string
  status?: string
  poll_url?: string
}

export interface ImageGenerationTaskResponse {
  task_id?: string
  taskId?: string
  id?: string
  status?: string
  raw_status?: string
  progress?: string
  fail_reason?: string
  data?: ImageGenerationResult
  [key: string]: unknown
}

// Configuration types
export interface PlaygroundConfig {
  model: string
  group: string
  temperature: number
  top_p: number
  max_tokens: number
  frequency_penalty: number
  presence_penalty: number
  seed: number | null
  stream: boolean
}

export interface ParameterEnabled {
  temperature: boolean
  top_p: boolean
  max_tokens: boolean
  frequency_penalty: boolean
  presence_penalty: boolean
  seed: boolean
}

// Model and group options
export interface ModelOption {
  label: string
  value: string
}

export interface GroupOption {
  label: string
  value: string
  ratio: number
  desc?: string
}
