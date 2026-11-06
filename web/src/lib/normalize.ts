import type { FeedItem, VideoDetail, VideoViewModel } from '@/types/api'

// Feed and detail endpoints return two different shapes (flat vs nested, different optional
// fields). Everything above this layer works with VideoViewModel only.

export function fromFeedItem(item: FeedItem): VideoViewModel {
  return {
    id: item.id,
    authorId: item.author_id,
    authorName: item.author?.nickname || item.author?.username || '未知作者',
    authorUsername: item.author?.username ?? '',
    authorAvatar: item.author?.avatar_url ?? '',
    title: item.title ?? '',
    description: item.description ?? '',
    coverUrl: item.cover_url ?? '',
    playUrl: item.play_url ?? '',
    durationMs: item.duration_ms ?? 0,
    publishedAt: item.published_at ?? '',
    likeCount: item.stats?.like_count ?? 0,
    commentCount: item.stats?.comment_count ?? 0,
    viewCount: item.stats?.view_count ?? 0,
    favoriteCount: item.stats?.favorite_count ?? 0,
    isLiked: Boolean(item.is_liked),
    isFollowed: Boolean(item.is_followed),
    isFavorited: Boolean(item.is_favorited),
    tags: item.tags ?? [],
    tagsKnown: Array.isArray(item.tags),
  }
}

export function fromVideoDetail(detail: VideoDetail): VideoViewModel {
  const video = detail.video
  return {
    id: video.id,
    authorId: video.author_id,
    authorName: detail.author?.nickname || detail.author?.username || '未知作者',
    authorUsername: detail.author?.username ?? '',
    authorAvatar: detail.author?.avatar_url ?? '',
    title: video.title ?? '',
    description: video.description ?? '',
    coverUrl: video.cover_url ?? '',
    playUrl: video.play_url ?? '',
    durationMs: video.duration_ms ?? 0,
    publishedAt: video.published_at ?? '',
    likeCount: detail.stats?.like_count ?? 0,
    commentCount: detail.stats?.comment_count ?? 0,
    viewCount: detail.stats?.view_count ?? 0,
    favoriteCount: detail.stats?.favorite_count ?? 0,
    isLiked: Boolean(detail.is_liked),
    isFollowed: false,
    isFavorited: Boolean(detail.is_favorited),
    tags: (video.tags ?? []).map((t) => t.display_name || t.name),
    tagsKnown: true,
  }
}
