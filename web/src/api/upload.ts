import type { UploadCoverResult, UploadVideoResult } from '@/types/api'
import { api } from './client'
import { session } from './tokens'

// Mirrors domainupload defaults and the handler-side cap: 5 MB chunks, 200 MB total.
export const CHUNK_SIZE = 5 * 1024 * 1024
export const SINGLE_UPLOAD_LIMIT = 20 * 1024 * 1024
export const MAX_VIDEO_SIZE = 200 * 1024 * 1024
export const MAX_COVER_SIZE = 10 * 1024 * 1024

export interface UploadProgress {
  loaded: number
  total: number
}

export const uploadApi = {
  /** Single-shot upload for clips under 20 MB; also used for covers. */
  uploadVideo(file: File, onProgress?: (p: UploadProgress) => void) {
    const form = new FormData()
    form.append('file', file)
    return xhrUpload<UploadVideoResult>('/videos/upload', form, onProgress)
  },

  uploadCover(file: File, onProgress?: (p: UploadProgress) => void) {
    const form = new FormData()
    form.append('file', file)
    return xhrUpload<UploadCoverResult>('/videos/cover', form, onProgress)
  },

  /**
   * Chunked, resumable upload. The whole-file SHA-256 is computed up front because the backend
   * uses it both as the session identity (dedupe/resume) and as the integrity check on merge.
   */
  async uploadVideoChunked(
    file: File,
    onProgress?: (p: UploadProgress) => void,
    signal?: AbortSignal,
  ): Promise<UploadVideoResult> {
    const totalChunks = Math.ceil(file.size / CHUNK_SIZE)
    const fileHash = await sha256Hex(await file.arrayBuffer())

    const init = await api.post<{
      upload_id: string
      uploaded_chunks: number[]
      total_chunks: number
      is_resumed: boolean
    }>('/videos/chunk/init', {
      file_name: file.name,
      file_size: file.size,
      chunk_size: CHUNK_SIZE,
      total_chunks: totalChunks,
      file_hash: fileHash,
    })

    const already = new Set(init.uploaded_chunks ?? [])
    let uploadedBytes = already.size * CHUNK_SIZE

    for (let index = 0; index < totalChunks; index += 1) {
      if (signal?.aborted) throw new DOMException('aborted', 'AbortError')
      if (already.has(index)) continue

      const start = index * CHUNK_SIZE
      const blob = file.slice(start, Math.min(start + CHUNK_SIZE, file.size))
      const chunkHash = await sha256Hex(await blob.arrayBuffer())

      const form = new FormData()
      form.append('upload_id', init.upload_id)
      form.append('chunk_index', String(index))
      form.append('chunk_hash', chunkHash)
      form.append('file', blob, `${file.name}.part${index}`)

      await withRetry(() => xhrUpload<{ chunk_index: number }>('/videos/chunk/upload', form), 3)
      uploadedBytes += blob.size
      onProgress?.({ loaded: Math.min(uploadedBytes, file.size), total: file.size })
    }

    const done = await api.post<{ upload_id: string; play_url: string; size: number }>(
      '/videos/chunk/complete',
      { upload_id: init.upload_id },
    )
    return { play_url: done.play_url, size: done.size }
  },
}

async function withRetry<T>(task: () => Promise<T>, attempts: number): Promise<T> {
  let lastError: unknown
  for (let i = 0; i < attempts; i += 1) {
    try {
      return await task()
    } catch (err) {
      lastError = err
      if (i < attempts - 1) await new Promise((r) => setTimeout(r, 300 * 2 ** i))
    }
  }
  throw lastError
}

/**
 * XHR instead of fetch: upload progress events are required by FR-PUB-02 and fetch has no
 * upload-progress support in browsers.
 */
function xhrUpload<T>(
  path: string,
  form: FormData,
  onProgress?: (p: UploadProgress) => void,
): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    xhr.open('POST', `/api/v1${path}`)
    const token = session.getAccessToken()
    if (token) xhr.setRequestHeader('Authorization', `Bearer ${token}`)
    xhr.upload.onprogress = (event) => {
      if (event.lengthComputable) onProgress?.({ loaded: event.loaded, total: event.total })
    }
    xhr.onload = () => {
      let payload: { data?: T; error?: { code: string; message: string } | null } | null = null
      try {
        payload = JSON.parse(xhr.responseText)
      } catch {
        payload = null
      }
      if (xhr.status >= 200 && xhr.status < 300 && payload?.data !== undefined) {
        resolve(payload.data as T)
        return
      }
      const code = payload?.error?.code ?? 'internal'
      reject(new Error(payload?.error?.message ?? `上传失败（${xhr.status}）${code}`))
    }
    xhr.onerror = () => reject(new Error('网络中断，上传失败'))
    xhr.send(form)
  })
}

async function sha256Hex(buffer: ArrayBuffer): Promise<string> {
  const digest = await crypto.subtle.digest('SHA-256', buffer)
  return Array.from(new Uint8Array(digest))
    .map((b) => b.toString(16).padStart(2, '0'))
    .join('')
}
