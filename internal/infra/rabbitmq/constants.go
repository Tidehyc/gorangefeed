package rabbitmq

// RabbitMQ Exchange and Queue Definitions for GoRangeFeed
const (
	// ExchangeEvents is the central topic exchange for domain and system events.
	ExchangeEvents = "exchange.gorangefeed.events"

	// QueueFollowingFeed is the consumer queue for processing following feed fanout.
	QueueFollowingFeed = "queue.gorangefeed.following_feed"

	// QueueHotRank is the consumer queue for processing hot rank engagement scoring.
	QueueHotRank = "queue.gorangefeed.hot_rank"

	// QueueNotification is the consumer queue for processing user notifications.
	QueueNotification = "queue.gorangefeed.notifications"
)

// Common Event Routing Keys
const (
	RoutingKeyVideoPublished           = "video.published"
	RoutingKeyInteractionLiked         = "interaction.liked"
	RoutingKeyInteractionUnliked       = "interaction.unliked"
	RoutingKeyInteractionCommented     = "interaction.commented"
	RoutingKeyInteractionCommentDelete = "interaction.comment_deleted"
	RoutingKeyRelationFollowed         = "relation.followed"
	RoutingKeyRelationUnfollowed       = "relation.unfollowed"
	RoutingKeyVideoViewed              = "video.viewed"
	RoutingKeyVideoCompleted           = "video.completed"
	RoutingKeyInteractionFavorited     = "interaction.favorited"
	RoutingKeyInteractionUnfavorited   = "interaction.unfavorited"
)
