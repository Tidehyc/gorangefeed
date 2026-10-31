package relation_test

import (
	"testing"

	"github.com/Tidehyc/gorangefeed/internal/domain/relation"
)

func TestFriendship(t *testing.T) {
	t.Run("mutual follow is friend", func(t *testing.T) {
		f := &relation.Friendship{
			IsFollowing: true,
			IsFollower:  true,
			IsFriend:    true,
		}
		if !f.IsFriend || !f.IsFollowing || !f.IsFollower {
			t.Errorf("expected mutual friend true")
		}
	})

	t.Run("single follow is not friend", func(t *testing.T) {
		f := &relation.Friendship{
			IsFollowing: true,
			IsFollower:  false,
			IsFriend:    false,
		}
		if f.IsFriend {
			t.Errorf("expected friend false")
		}
	})
}
