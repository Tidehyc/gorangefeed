import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { authApi } from '@/api/auth'
import { ApiError } from '@/api/client'
import { session } from '@/api/tokens'
import { useAuthStore } from '@/stores/auth'
import { toast } from '@/stores/toast'
import { PageHeader } from '@/components/PageHeader'
import { Avatar } from '@/components/ui/Avatar'
import { Button } from '@/components/ui/Button'

export function SettingsPage() {
  const navigate = useNavigate()
  const user = useAuthStore((state) => state.user)
  const patchUser = useAuthStore((state) => state.patchUser)
  const logout = useAuthStore((state) => state.logout)

  const [nickname, setNickname] = useState(user?.nickname ?? '')
  const [bio, setBio] = useState(user?.bio ?? '')
  const [saving, setSaving] = useState(false)
  const [uploading, setUploading] = useState(false)

  const [oldPassword, setOldPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [passwordBusy, setPasswordBusy] = useState(false)

  const [deleteOpen, setDeleteOpen] = useState(false)
  const [deletePassword, setDeletePassword] = useState('')
  const [deleteConfirm, setDeleteConfirm] = useState('')
  const [deleteBusy, setDeleteBusy] = useState(false)

  const saveProfile = async () => {
    if (!nickname.trim()) {
      toast.error('昵称不能为空')
      return
    }
    setSaving(true)
    try {
      const updated = await authApi.updateProfile(nickname.trim(), bio.trim())
      patchUser({ nickname: updated.nickname, bio: updated.bio })
      toast.success('资料已保存')
    } catch (err) {
      toast.error(err instanceof ApiError ? err.friendlyMessage : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const uploadAvatar = async (file: File | undefined) => {
    if (!file) return
    setUploading(true)
    try {
      const result = await authApi.uploadAvatar(file)
      patchUser({ avatar_url: result.avatar_url })
      toast.success('头像已更新')
    } catch (err) {
      toast.error(err instanceof ApiError ? err.friendlyMessage : '头像上传失败')
    } finally {
      setUploading(false)
    }
  }

  const changePassword = async () => {
    if (newPassword.length < 8 || !/[A-Za-z]/.test(newPassword) || !/\d/.test(newPassword)) {
      toast.error('新密码需至少 8 位，且包含字母和数字')
      return
    }
    setPasswordBusy(true)
    try {
      await authApi.changePassword(oldPassword, newPassword)
      // The server retired every token issued before the change, including this one.
      session.clear()
      toast.success('密码已修改，请重新登录')
      navigate('/login', { replace: true })
    } catch (err) {
      toast.error(err instanceof ApiError ? err.friendlyMessage : '修改失败')
    } finally {
      setPasswordBusy(false)
    }
  }

  const deleteAccount = async () => {
    if (deleteConfirm.trim() !== (user?.nickname ?? '')) {
      toast.error('请输入完整昵称以确认')
      return
    }
    setDeleteBusy(true)
    try {
      await authApi.deleteAccount(deletePassword)
      session.clear()
      toast.success('账户已注销')
      navigate('/login', { replace: true })
    } catch (err) {
      toast.error(err instanceof ApiError ? err.friendlyMessage : '注销失败')
      setDeleteBusy(false)
    }
  }

  return (
    <div className="mx-auto min-h-[100dvh] w-full max-w-[640px] pb-24">
      <PageHeader title="账号设置" />

      <div className="flex flex-col gap-4 px-4 py-4">
        <section className="rounded-2xl bg-white/[0.04] p-4">
          <h2 className="mb-3 text-[13px] font-medium text-mist-300">头像</h2>
          <div className="flex items-center gap-4">
            <Avatar src={user?.avatar_url} name={user?.nickname} size={64} />
            <label className="cursor-pointer rounded-full border border-white/20 px-4 py-1.5 text-[12px] text-mist-200 hover:bg-white/5">
              {uploading ? '上传中…' : '更换头像'}
              <input
                type="file"
                accept="image/*"
                className="hidden"
                onChange={(event) => void uploadAvatar(event.target.files?.[0])}
              />
            </label>
          </div>
        </section>

        <section className="flex flex-col gap-3 rounded-2xl bg-white/[0.04] p-4">
          <h2 className="text-[13px] font-medium text-mist-300">资料</h2>
          <label className="flex flex-col gap-1.5">
            <span className="text-[12px] text-mist-500">昵称</span>
            <input
              value={nickname}
              onChange={(event) => setNickname(event.target.value)}
              maxLength={64}
              className="h-10 rounded-xl border border-white/10 bg-white/5 px-3 text-[14px] focus:border-brand/60"
            />
          </label>
          <label className="flex flex-col gap-1.5">
            <span className="text-[12px] text-mist-500">简介</span>
            <textarea
              value={bio}
              onChange={(event) => setBio(event.target.value)}
              maxLength={255}
              rows={2}
              className="resize-none rounded-xl border border-white/10 bg-white/5 px-3 py-2 text-[14px] focus:border-brand/60"
            />
          </label>
          <div className="flex items-center justify-between">
            <span className="text-[11px] text-mist-700">@{user?.username}</span>
            <Button size="sm" onClick={() => void saveProfile()} loading={saving}>
              保存
            </Button>
          </div>
        </section>

        <section className="flex flex-col gap-3 rounded-2xl bg-white/[0.04] p-4">
          <h2 className="text-[13px] font-medium text-mist-300">修改密码</h2>
          <input
            type="password"
            value={oldPassword}
            onChange={(event) => setOldPassword(event.target.value)}
            placeholder="当前密码"
            autoComplete="current-password"
            className="h-10 rounded-xl border border-white/10 bg-white/5 px-3 text-[14px] focus:border-brand/60"
          />
          <input
            type="password"
            value={newPassword}
            onChange={(event) => setNewPassword(event.target.value)}
            placeholder="新密码（至少 8 位，含字母与数字）"
            autoComplete="new-password"
            className="h-10 rounded-xl border border-white/10 bg-white/5 px-3 text-[14px] focus:border-brand/60"
          />
          <Button
            size="sm"
            variant="ghost"
            onClick={() => void changePassword()}
            loading={passwordBusy}
            disabled={!oldPassword || !newPassword}
            className="self-end"
          >
            修改并重新登录
          </Button>
          <p className="text-[11px] leading-4 text-mist-700">
            修改成功后，所有设备上的旧登录凭证会立即失效。
          </p>
        </section>

        <section className="flex flex-col gap-2 rounded-2xl bg-white/[0.04] p-4">
          <h2 className="text-[13px] font-medium text-mist-300">其他</h2>
          <Link
            to={`/user/${user?.id ?? 0}/followings`}
            className="rounded-xl px-1 py-2 text-[13px] text-mist-300 hover:text-mist-100"
          >
            我的关注与粉丝
          </Link>
          <button
            type="button"
            onClick={() => void logout().then(() => navigate('/login', { replace: true }))}
            className="rounded-xl px-1 py-2 text-left text-[13px] text-mist-300 hover:text-mist-100"
          >
            退出登录
          </button>
        </section>

        <section className="rounded-2xl border border-red-500/20 bg-red-500/[0.06] p-4">
          <h2 className="text-[13px] font-medium text-red-300">危险操作</h2>
          <p className="mt-1 text-[12px] leading-5 text-mist-500">
            注销后账户无法恢复：昵称会被匿名化，所有作品会从推荐流隐藏，评论作者显示为「已注销用户」。
          </p>
          {!deleteOpen ? (
            <Button variant="danger" size="sm" className="mt-3" onClick={() => setDeleteOpen(true)}>
              注销账户
            </Button>
          ) : (
            <div className="mt-3 flex flex-col gap-2">
              <input
                type="password"
                value={deletePassword}
                onChange={(event) => setDeletePassword(event.target.value)}
                placeholder="输入登录密码"
                className="h-10 rounded-xl border border-white/10 bg-white/5 px-3 text-[14px]"
              />
              <input
                value={deleteConfirm}
                onChange={(event) => setDeleteConfirm(event.target.value)}
                placeholder={`输入昵称「${user?.nickname ?? ''}」以确认`}
                className="h-10 rounded-xl border border-white/10 bg-white/5 px-3 text-[14px]"
              />
              <div className="flex gap-2">
                <Button
                  variant="danger"
                  size="sm"
                  loading={deleteBusy}
                  disabled={!deletePassword || !deleteConfirm}
                  onClick={() => void deleteAccount()}
                >
                  确认注销
                </Button>
                <Button variant="ghost" size="sm" onClick={() => setDeleteOpen(false)}>
                  取消
                </Button>
              </div>
            </div>
          )}
        </section>
      </div>
    </div>
  )
}
