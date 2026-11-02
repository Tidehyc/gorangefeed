package notification_test

import (
	"context"
	"errors"
	"testing"

	domainaccount "github.com/Tidehyc/gorangefeed/internal/domain/account"
	servicenotification "github.com/Tidehyc/gorangefeed/internal/service/notification"
)

type mockAccountFinder struct {
	byUsername map[string]*domainaccount.User
	byID       map[int64]*domainaccount.User
}

func (m *mockAccountFinder) FindByUsername(ctx context.Context, username string) (*domainaccount.User, error) {
	if u, ok := m.byUsername[username]; ok {
		return u, nil
	}
	return nil, errors.New("user not found")
}

func (m *mockAccountFinder) FindByID(ctx context.Context, id int64) (*domainaccount.User, error) {
	if u, ok := m.byID[id]; ok {
		return u, nil
	}
	return nil, errors.New("user not found")
}

func TestExtractUsernames(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty text",
			input:    "",
			expected: nil,
		},
		{
			name:     "no mentions",
			input:    "This is just a regular comment with no mentions.",
			expected: nil,
		},
		{
			name:     "single mention",
			input:    "Great video @alice!",
			expected: []string{"alice"},
		},
		{
			name:     "multiple mentions with punctuation",
			input:    "Hey @alice, check with @bob_123 and @charlie?",
			expected: []string{"alice", "bob_123", "charlie"},
		},
		{
			name:     "deduplicate mentions",
			input:    "@alice please check @alice and @bob and again @alice",
			expected: []string{"alice", "bob"},
		},
		{
			name:     "ignore too short usernames",
			input:    "Hey @a and @ab are too short, but @abc is valid!",
			expected: []string{"abc"},
		},
		{
			name:     "capped at 10 mentions",
			input:    "@u01 @u02 @u03 @u04 @u05 @u06 @u07 @u08 @u09 @u10 @u11 @u12",
			expected: []string{"u01", "u02", "u03", "u04", "u05", "u06", "u07", "u08", "u09", "u10"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := servicenotification.ExtractUsernames(tt.input)
			if len(got) != len(tt.expected) {
				t.Fatalf("expected %d usernames, got %d: %v", len(tt.expected), len(got), got)
			}
			for i, v := range tt.expected {
				if got[i] != v {
					t.Fatalf("expected at %d: %s, got %s", i, v, got[i])
				}
			}
		})
	}
}

func TestMentionParser_ParseMentions(t *testing.T) {
	mockFinder := &mockAccountFinder{
		byUsername: map[string]*domainaccount.User{
			"actor_user":  {ID: 101, Username: "actor_user"},
			"author_user": {ID: 102, Username: "author_user"},
			"alice":       {ID: 103, Username: "alice"},
			"bob":         {ID: 104, Username: "bob"},
			"charlie":     {ID: 105, Username: "charlie"},
		},
		byID: map[int64]*domainaccount.User{
			101: {ID: 101, Username: "actor_user"},
			102: {ID: 102, Username: "author_user"},
			103: {ID: 103, Username: "alice"},
			104: {ID: 104, Username: "bob"},
			105: {ID: 105, Username: "charlie"},
		},
	}

	parser := servicenotification.NewMentionParser(mockFinder)
	ctx := context.Background()

	t.Run("filter self-mention and author", func(t *testing.T) {
		text := "Hello @actor_user @author_user @alice @bob and @non_existent"
		actorID := int64(101)
		authorID := int64(102)

		recipients, err := parser.ParseMentions(ctx, text, actorID, authorID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(recipients) != 2 {
			t.Fatalf("expected 2 recipients (alice and bob), got %d: %v", len(recipients), recipients)
		}

		if recipients[0] != 103 || recipients[1] != 104 {
			t.Fatalf("expected [103, 104], got %v", recipients)
		}
	})

	t.Run("empty text and no mentions", func(t *testing.T) {
		res, err := parser.ParseMentions(ctx, "", 101, 102)
		if err != nil || res != nil {
			t.Fatalf("expected nil for empty text, got %v, %v", res, err)
		}

		res, err = parser.ParseMentions(ctx, "no mentions here", 101, 102)
		if err != nil || res != nil {
			t.Fatalf("expected nil for no mentions, got %v, %v", res, err)
		}
	})
}
