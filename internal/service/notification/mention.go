package notification

import (
	"context"
	"regexp"

	domainaccount "github.com/Tidehyc/gorangefeed/internal/domain/account"
)

// MentionRegex matches valid @username mentions.
// Usernames are 3 to 32 alphanumeric characters and underscores.
var MentionRegex = regexp.MustCompile(`@([a-zA-Z0-9_]{3,32})`)

// MaxMentionsPerComment is the upper bound on mentioned users processed per comment to prevent spam.
const MaxMentionsPerComment = 10

// AccountFinder abstracts account lookups by username or ID for notification services.
type AccountFinder interface {
	FindByUsername(ctx context.Context, username string) (*domainaccount.User, error)
	FindByID(ctx context.Context, id int64) (*domainaccount.User, error)
}

// MentionParser parses and resolves @username mentions from comment text.
type MentionParser struct {
	accountFinder AccountFinder
}

// NewMentionParser creates a new MentionParser instance.
func NewMentionParser(accountFinder AccountFinder) *MentionParser {
	return &MentionParser{
		accountFinder: accountFinder,
	}
}

// ExtractUsernames extracts unique valid usernames from text, capped at MaxMentionsPerComment (10).
func ExtractUsernames(text string) []string {
	if text == "" {
		return nil
	}

	matches := MentionRegex.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]bool, len(matches))
	result := make([]string, 0, len(matches))

	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		username := m[1]
		if !seen[username] {
			seen[username] = true
			result = append(result, username)
			if len(result) >= MaxMentionsPerComment {
				break
			}
		}
	}
	return result
}

// ParseMentions extracts usernames, looks up their user IDs, and filters out self-mentions and authorID.
func (p *MentionParser) ParseMentions(ctx context.Context, text string, actorID, authorID int64) ([]int64, error) {
	usernames := ExtractUsernames(text)
	if len(usernames) == 0 || p.accountFinder == nil {
		return nil, nil
	}

	seenUID := make(map[int64]bool)
	var recipientIDs []int64

	for _, uname := range usernames {
		user, err := p.accountFinder.FindByUsername(ctx, uname)
		if err != nil || user == nil {
			// Ignore non-existent users or query failures gracefully
			continue
		}

		// Filter out self-mention
		if user.ID == actorID {
			continue
		}

		// Filter out video author (already receives TypeComment notification)
		if user.ID == authorID {
			continue
		}

		if !seenUID[user.ID] {
			seenUID[user.ID] = true
			recipientIDs = append(recipientIDs, user.ID)
		}
	}

	return recipientIDs, nil
}
