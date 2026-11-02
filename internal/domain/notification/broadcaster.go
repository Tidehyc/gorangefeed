package notification

import "context"

// Broadcaster abstracts the notification message publishing and subscription engine across instances.
type Broadcaster interface {
	// Publish broadcasts a notification message to all listening instances.
	Publish(ctx context.Context, notif *Notification) error

	// Subscribe subscribes to incoming broadcast notifications across the cluster.
	Subscribe(ctx context.Context) (<-chan *Notification, error)

	// Close closes the broadcaster connection and any background subscriptions.
	Close() error
}
