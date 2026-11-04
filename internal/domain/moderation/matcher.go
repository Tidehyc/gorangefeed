package moderation

// MatchResult encapsulates the outcome of a sensitive keyword scan.
type MatchResult struct {
	MatchedWords []string `json:"matched_words"`
	RiskScore    int      `json:"risk_score"`
}

// SensitiveMatcher defines the contract for scanning text against prohibited keywords.
type SensitiveMatcher interface {
	// Match scans the input content (e.g. title + description) and returns matching words and overall risk score.
	Match(text string) MatchResult
}
