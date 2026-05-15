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

const markdownImageReferenceRegex =
  /!\[([^\]]*)]\((data:image\/[A-Za-z0-9.+-]+;base64,[A-Za-z0-9+/=\r\n]+|\/pg\/(?:public\/)?images\/generations\/[^)\s]+|https?:\/\/[^)\s]+)\)/g

const rawPlaygroundImageReferenceRegex =
  /(https?:\/\/[^)\s]+\/pg\/(?:public\/)?images\/generations\/[^)\s.,;，。；]+|\/pg\/(?:public\/)?images\/generations\/[^)\s.,;，。；]+)/g

export interface ImageReferenceParseResult {
  text: string
  imageUrls: string[]
}

export function normalizeImageReferenceUrl(url: string): string {
  return String(url || '').trim().replace(/[\r\n]/g, '')
}

export function toAbsoluteImageReferenceUrl(url: string): string {
  const normalized = normalizeImageReferenceUrl(url)
  if (!normalized.startsWith('/')) {
    return normalized
  }
  if (typeof window === 'undefined' || !window.location?.origin) {
    return normalized
  }
  return `${window.location.origin}${normalized}`
}

export function dedupeImageReferenceUrls(urls: string[]): string[] {
  const seen = new Set<string>()
  const result: string[] = []

  urls.forEach((url) => {
    const normalized = normalizeImageReferenceUrl(url)
    if (!normalized || seen.has(normalized)) return
    seen.add(normalized)
    result.push(normalized)
  })

  return result
}

export function parseImageReferencesFromText(
  content: string
): ImageReferenceParseResult {
  if (!content.trim()) {
    return { text: '', imageUrls: [] }
  }

  const imageUrls: string[] = []
  const text = content
    .replace(
      markdownImageReferenceRegex,
      (_match, _alt: string, url: string) => {
        imageUrls.push(normalizeImageReferenceUrl(url))
        return '\n'
      }
    )
    .replace(rawPlaygroundImageReferenceRegex, (_match, url: string) => {
      imageUrls.push(normalizeImageReferenceUrl(url))
      return '\n'
    })
    .replace(/\n{3,}/g, '\n\n')
    .trim()

  return {
    text,
    imageUrls: dedupeImageReferenceUrls(imageUrls),
  }
}
