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
import { api } from '@/lib/api'
import { API_ENDPOINTS } from './constants'
import type {
  ChatCompletionRequest,
  ChatCompletionResponse,
  ImageGenerationRequest,
  ImageGenerationSubmitResponse,
  ImageGenerationTaskResponse,
  ModelOption,
  GroupOption,
} from './types'

/**
 * Send chat completion request (non-streaming)
 */
export async function sendChatCompletion(
  payload: ChatCompletionRequest
): Promise<ChatCompletionResponse> {
  const res = await api.post(API_ENDPOINTS.CHAT_COMPLETIONS, payload, {
    skipErrorHandler: true,
  } as Record<string, unknown>)
  return res.data
}

/**
 * Send image generation request
 */
export async function sendImageGeneration(
  payload: ImageGenerationRequest,
  signal?: AbortSignal
): Promise<ImageGenerationSubmitResponse> {
  const res = await api.post(API_ENDPOINTS.IMAGE_GENERATIONS, payload, {
    signal,
    skipErrorHandler: true,
  } as Record<string, unknown>)
  return res.data
}

/**
 * Get image generation task status/result
 */
export async function getImageGenerationTask(
  taskId: string,
  signal?: AbortSignal
): Promise<ImageGenerationTaskResponse> {
  const res = await api.get(
    `${API_ENDPOINTS.IMAGE_GENERATIONS}/${encodeURIComponent(taskId)}`,
    {
      signal,
      disableDuplicate: true,
      skipErrorHandler: true,
    } as Record<string, unknown>
  )
  return res.data
}

/**
 * Get user available models
 */
export async function getUserModels(): Promise<ModelOption[]> {
  return getUserModelsByGroup()
}

/**
 * Get user available models for a selected group
 */
export async function getUserModelsByGroup(
  group?: string
): Promise<ModelOption[]> {
  const query = group ? `?group=${encodeURIComponent(group)}` : ''
  const res = await api
    .get(`${API_ENDPOINTS.USER_MODELS}${query}`, {
      skipErrorHandler: true,
    } as Record<string, unknown>)
    .catch(() => null)

  if (!res) {
    return []
  }

  const { data } = res

  if (!data.success || !Array.isArray(data.data)) {
    return []
  }

  return data.data.map((model: string) => ({
    label: model,
    value: model,
  }))
}

/**
 * Get user groups
 */
export async function getUserGroups(): Promise<GroupOption[]> {
  const res = await api
    .get(API_ENDPOINTS.USER_GROUPS, {
      skipErrorHandler: true,
    } as Record<string, unknown>)
    .catch(() => null)

  if (!res) {
    return []
  }

  const { data } = res

  if (!data.success || !data.data) {
    return []
  }

  const groupData = data.data as Record<string, { desc: string; ratio: number }>

  // label is for button display (name only); desc is for dropdown content
  return Object.entries(groupData).map(([group, info]) => ({
    label: group,
    value: group,
    ratio: info.ratio,
    desc: info.desc,
  }))
}
