import { useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { ApiError } from '@/api/client'
import { videoApi } from '@/api/video'
import {
  MAX_COVER_SIZE,
  MAX_VIDEO_SIZE,
  SINGLE_UPLOAD_LIMIT,
  uploadApi,
} from '@/api/upload'
import { newIdempotencyKey } from '@/lib/format'
import { toast } from '@/stores/toast'
import { PageHeader } from '@/components/PageHeader'
import { Button } from '@/components/ui/Button'
import { Spinner } from '@/components/ui/Spinner'

export function PublishPage() {
  const navigate = useNavigate()
  const videoInput = useRef<HTMLInputElement>(null)
  const coverInput = useRef<HTMLInputElement>(null)

  const [file, setFile] = useState<File | null>(null)
  const [cover, setCover] = useState<File | null>(null)
  const [durationMs, setDurationMs] = useState(0)
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [tagText, setTagText] = useState('')
  const [progress, setProgress] = useState(0)
  const [busy, setBusy] = useState(false)
  const [stage, setStage] = useState('')

  const pickVideo = (picked: File | undefined) => {
    if (!picked) return
    if (!picked.type.startsWith('video/')) {
      toast.error('请选择视频文件')
      return
    }
    if (picked.size > MAX_VIDEO_SIZE) {
      toast.error('视频不能超过 200MB')
      return
    }
    setFile(picked)
    setProgress(0)

    // Duration is metadata the backend stores for display; read it locally instead of uploading
    // first and asking later.
    const url = URL.createObjectURL(picked)
    const probe = document.createElement('video')
    probe.preload = 'metadata'
    probe.onloadedmetadata = () => {
      setDurationMs(Math.round((probe.duration || 0) * 1000))
      URL.revokeObjectURL(url)
    }
    probe.onerror = () => URL.revokeObjectURL(url)
    probe.src = url
  }

  const pickCover = (picked: File | undefined) => {
    if (!picked) return
    if (!picked.type.startsWith('image/')) {
      toast.error('封面需要是图片')
      return
    }
    if (picked.size > MAX_COVER_SIZE) {
      toast.error('封面不能超过 10MB')
      return
    }
    setCover(picked)
  }

  const submit = async () => {
    if (!file) {
      toast.error('请先选择视频')
      return
    }
    if (!title.trim()) {
      toast.error('请填写标题')
      return
    }
    setBusy(true)
    try {
      setStage('上传视频')
      const uploaded =
        file.size <= SINGLE_UPLOAD_LIMIT
          ? await uploadApi.uploadVideo(file, (p) => setProgress(Math.round((p.loaded / p.total) * 100)))
          : await uploadApi.uploadVideoChunked(file, (p) =>
              setProgress(Math.round((p.loaded / p.total) * 100)),
            )

      let coverUrl = ''
      if (cover) {
        setStage('上传封面')
        const result = await uploadApi.uploadCover(cover)
        coverUrl = result.cover_url
      }

      setStage('发布')
      const tags = tagText
        .split(/[\s,#，、]+/)
        .map((tag) => tag.trim())
        .filter(Boolean)
        .slice(0, 5)

      const detail = await videoApi.publish(
        {
          title: title.trim(),
          description: description.trim(),
          play_url: uploaded.play_url,
          cover_url: coverUrl,
          duration_ms: durationMs,
          tags,
        },
        newIdempotencyKey(),
      )

      toast.success('发布成功')
      navigate(`/video/${detail.video.id}`, { replace: true })
    } catch (err) {
      toast.error(err instanceof ApiError ? err.friendlyMessage : (err as Error).message || '发布失败')
      setBusy(false)
      setStage('')
      setProgress(0)
    }
  }

  return (
    <div className="mx-auto min-h-[100dvh] w-full max-w-[640px] pb-24">
      <PageHeader title="发布作品" />

      <div className="flex flex-col gap-5 px-4 pt-4">
        <button
          type="button"
          onClick={() => videoInput.current?.click()}
          className="flex aspect-video w-full flex-col items-center justify-center gap-2 rounded-2xl border border-dashed border-white/15 bg-white/[0.03] text-mist-500 hover:border-brand/50"
        >
          {file ? (
            <>
              <span className="text-[14px] text-mist-100">{file.name}</span>
              <span className="text-[12px]">
                {(file.size / 1024 / 1024).toFixed(1)} MB ·{' '}
                {file.size > SINGLE_UPLOAD_LIMIT ? '分片上传（5MB/片）' : '单文件上传'}
              </span>
              <span className="text-[12px] text-brand">点击重新选择</span>
            </>
          ) : (
            <>
              <span className="text-3xl">＋</span>
              <span className="text-[14px]">选择视频</span>
              <span className="text-[12px]">MP4，≤200MB；≤20MB 走直传，更大自动分片</span>
            </>
          )}
        </button>
        <input
          ref={videoInput}
          type="file"
          accept="video/mp4,video/*"
          className="hidden"
          onChange={(event) => pickVideo(event.target.files?.[0])}
        />

        <label className="flex flex-col gap-1.5">
          <span className="text-[12px] text-mist-500">标题（必填）</span>
          <input
            value={title}
            onChange={(event) => setTitle(event.target.value)}
            maxLength={128}
            placeholder="给作品起一个有信息量的标题"
            className="h-11 rounded-xl border border-white/10 bg-white/5 px-3 text-[15px] focus:border-brand/60"
          />
        </label>

        <label className="flex flex-col gap-1.5">
          <span className="text-[12px] text-mist-500">描述</span>
          <textarea
            value={description}
            onChange={(event) => setDescription(event.target.value)}
            maxLength={1000}
            rows={3}
            placeholder="讲讲这条视频背后的故事"
            className="resize-none rounded-xl border border-white/10 bg-white/5 px-3 py-2 text-[14px] leading-6 focus:border-brand/60"
          />
        </label>

        <label className="flex flex-col gap-1.5">
          <span className="text-[12px] text-mist-500">话题（最多 5 个，空格或逗号分隔）</span>
          <input
            value={tagText}
            onChange={(event) => setTagText(event.target.value)}
            placeholder="旅行 海边 日落"
            className="h-11 rounded-xl border border-white/10 bg-white/5 px-3 text-[15px] focus:border-brand/60"
          />
        </label>

        <button
          type="button"
          onClick={() => coverInput.current?.click()}
          className="flex items-center justify-between rounded-xl border border-white/10 bg-white/5 px-3 py-2.5 text-left"
        >
          <span className="text-[13px] text-mist-300">{cover ? cover.name : '选择封面（可选）'}</span>
          <span className="text-[12px] text-brand">jpg / png / webp ≤10MB</span>
        </button>
        <input
          ref={coverInput}
          type="file"
          accept="image/*"
          className="hidden"
          onChange={(event) => pickCover(event.target.files?.[0])}
        />

        {busy && (
          <div className="flex flex-col gap-2 rounded-xl bg-white/5 p-3">
            <div className="flex items-center justify-between text-[12px] text-mist-400">
              <span className="flex items-center gap-2">
                <Spinner size={12} /> {stage}
              </span>
              <span>{progress}%</span>
            </div>
            <div className="h-1.5 overflow-hidden rounded-full bg-white/10">
              <div className="h-full rounded-full bg-brand transition-all" style={{ width: `${progress}%` }} />
            </div>
          </div>
        )}

        <Button size="lg" block onClick={() => void submit()} loading={busy} disabled={!file || !title.trim()}>
          发布
        </Button>

        <p className="pb-6 text-center text-[11px] leading-5 text-mist-700">
          发布即表示你确认上传内容拥有合法使用权。若作品被机审隐藏，它会暂时从个人主页消失。
        </p>
      </div>
    </div>
  )
}
