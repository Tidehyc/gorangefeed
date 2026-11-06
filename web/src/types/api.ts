// Wire types mirroring the Go DTOs one-to-one. Nothing here is invented: every field name was
// read off the handler structs, and optional fields are marked optional for the reason the
// backend omits them (different endpoints, different shapes).

export interface ApiErrorPayload {
  code: string
  message: string
}

export interface Envelope<T> {
  data: T | null
  error: ApiErrorPayload | null
}

export interface TokenPair {
  access_token: string
  refresh_token: string
  expires_in: number
  token_type: string
}

export interface UserStats {
  following_count: number
  follower_count: number
  work_count: number
  total_likes: number
}

export interface UserProfile {
  id: number
  username: string
  nickname: string
  avatar_url: string
  bio: string
  status: number
  created_at: string
  stats: UserStats
}

export interface PublicUserProfile {
  id: number
  nickname: string
  avatar_url: string
  bio: string
  stats: UserStats
}

export interface LoginResponse {
  tokens: TokenPair
  user: UserProfile
}

export interface FeedAuthor {
  id: number
  username: string
  nickname: string
  avatar_url: string
}

export interface FeedStat {
  video_id: number
  like_count: number
  comment_count: number
  view_count: number
  favorite_count: number
}

export interface FeedItem {
  id: number
  author_id: number
  title: string
  description: string
  cover_url: string
  play_url: string
  duration_ms: number
  published_at: string
  author: FeedAuthor
  stats: FeedStat
  is_liked: boolean
  is_followed: boolean
  is_favorited: boolean
  tags?: string[]
}

export interface FeedPage {
  items: FeedItem[]
  // The backend mixes numeric IDs and base64/opaque strings across scenes; treat as opaque.
  next_cursor: number | string
  has_more: boolean
}

export interface VideoTag {
  name: string
  display_name: string
}

export interface Video {
  id: number
  author_id: number
  title: string
  description: string
  cover_url: string
  play_url: string
  duration_ms: number
  status: number
  published_at: string
  tags?: VideoTag[]
}

export interface VideoStats {
  video_id: number
  like_count: number
  comment_count: number
  view_count: number
  favorite_count: number
}

export interface AuthorProfile {
  id: number
  username: string
  nickname: string
  avatar_url: string
}

export interface VideoDetail {
  video: Video
  stats: VideoStats
  author: AuthorProfile
  is_liked: boolean
  is_favorited?: boolean
}

export interface AuthorVideosPage {
  videos: VideoDetail[]
  next_cursor: number
  has_more: boolean
}

export interface Comment {
  id: number
  video_id: number
  user_id: number
  parent_id: number
  content: string
  like_count: number
  created_at: string
}

export interface CommentAuthor {
  id: number
  username: string
  nickname: string
  avatar_url: string
}

export interface CommentDetail {
  comment: Comment
  author: CommentAuthor
  is_liked: boolean
}

export interface CommentPage {
  items: CommentDetail[]
  next_cursor: string
  has_more: boolean
}

export interface RelationUser {
  id: number
  username: string
  nickname: string
  avatar_url: string
  bio: string
  is_friend: boolean
}

export interface RelationPage {
  items: RelationUser[]
  next_cursor: number
  has_more: boolean
}

export interface Friendship {
  is_following: boolean
  is_follower: boolean
  is_friend: boolean
}

export interface NotificationActor {
  id: number
  username: string
  nickname: string
  avatar_url: string
}

export interface NotificationItem {
  id: number
  recipient_id: number
  actor_id: number
  type: string
  target_type: string
  target_id: number
  title: string
  content: string
  is_read: boolean
  created_at: string
  actor?: NotificationActor
}

export interface NotificationPage {
  items: NotificationItem[]
  next_cursor: string
  has_more: boolean
}

export interface UploadVideoResult {
  play_url: string
  size: number
}

export interface UploadCoverResult {
  cover_url: string
}

// FeedItem and VideoDetail normalize into one view model so cards do not need to know which
// endpoint they came from.
export interface VideoViewModel {
  id: number
  authorId: number
  authorName: string
  authorUsername: string
  authorAvatar: string
  title: string
  description: string
  coverUrl: string
  playUrl: string
  durationMs: number
  publishedAt: string
  likeCount: number
  commentCount: number
  viewCount: number
  favoriteCount: number
  isLiked: boolean
  isFollowed: boolean
  isFavorited: boolean
  tags: string[]
  // Kept so the detail page can tell "not loaded yet" from "loaded, no tags".
  tagsKnown: boolean
}
