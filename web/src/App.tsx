import { useEffect } from 'react'
import { Route, Routes, useNavigate } from 'react-router-dom'
import { session } from '@/api/tokens'
import { AppShell } from '@/components/AppShell'
import { RequireAuth } from '@/components/RequireAuth'
import { FeedPage } from '@/pages/FeedPage'
import { FollowListPage } from '@/pages/FollowListPage'
import { LoginPage } from '@/pages/LoginPage'
import { MePage } from '@/pages/MePage'
import { NotFoundPage } from '@/pages/NotFoundPage'
import { NotificationsPage } from '@/pages/NotificationsPage'
import { PublishPage } from '@/pages/PublishPage'
import { SettingsPage } from '@/pages/SettingsPage'
import { TagPage } from '@/pages/TagPage'
import { UserPage } from '@/pages/UserPage'
import { VideoDetailPage } from '@/pages/VideoDetailPage'
import { useAuthStore } from '@/stores/auth'
import { toast } from '@/stores/toast'

export function App() {
  const bootstrap = useAuthStore((state) => state.bootstrap)
  const navigate = useNavigate()

  useEffect(() => {
    void bootstrap()
  }, [bootstrap])

  // A refresh that fails for good must land somewhere actionable instead of leaving the UI in a
  // permanently "signed in but 401" state.
  useEffect(() => {
    session.onExpire(() => {
      toast.error('登录状态已失效，请重新登录')
      navigate('/login', { replace: true })
    })
    return () => session.onExpire(null)
  }, [navigate])

  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />

      <Route element={<AppShell />}>
        <Route index element={<FeedPage />} />
        <Route path="/video/:id" element={<VideoDetailPage />} />
        <Route path="/user/:id" element={<UserPage />} />
        <Route path="/user/:id/followers" element={<FollowListPage kind="followers" />} />
        <Route path="/user/:id/followings" element={<FollowListPage kind="followings" />} />
        <Route path="/tag/:tag" element={<TagPage />} />

        <Route
          path="/publish"
          element={
            <RequireAuth>
              <PublishPage />
            </RequireAuth>
          }
        />
        <Route
          path="/me"
          element={
            <RequireAuth>
              <MePage />
            </RequireAuth>
          }
        />
        <Route
          path="/settings"
          element={
            <RequireAuth>
              <SettingsPage />
            </RequireAuth>
          }
        />
        <Route
          path="/notifications"
          element={
            <RequireAuth>
              <NotificationsPage />
            </RequireAuth>
          }
        />

        <Route path="*" element={<NotFoundPage />} />
      </Route>
    </Routes>
  )
}
