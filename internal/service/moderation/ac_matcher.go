package moderation

import (
	"strings"
	"sync"

	domainmoderation "github.com/Tidehyc/gorangefeed/internal/domain/moderation"
)

type acPattern struct {
	word      string
	riskScore int
}

type acNode struct {
	children map[rune]*acNode
	fail     *acNode
	patterns []acPattern
}

func newACNode() *acNode {
	return &acNode{
		children: make(map[rune]*acNode),
		patterns: make([]acPattern, 0),
	}
}

// ACMatcher implements domainmoderation.SensitiveMatcher using the Aho-Corasick algorithm.
type ACMatcher struct {
	mu   sync.RWMutex
	root *acNode
}

// NewACMatcher creates an initialized ACMatcher with default high-risk and medium-risk vocabularies.
func NewACMatcher() *ACMatcher {
	m := &ACMatcher{
		root: newACNode(),
	}
	m.loadDefaultDictionary()
	m.Build()
	return m
}

func (m *ACMatcher) loadDefaultDictionary() {
	// High-risk prohibited keywords (score 80~100) -> auto rejected
	highRisk := []string{"违禁品", "赌博", "枪支", "毒品", "暴恐", "反动", "炸弹", "色情", "诈骗", "假钞"}
	for _, w := range highRisk {
		m.AddWord(w, 85)
	}

	// Medium-risk sensitive keywords (score 30~50) -> pending human review
	mediumRisk := []string{"兼职刷单", "加微信", "加v信", "私聊", "引流", "高仿", "办证", "免费领苹果", "日入过万"}
	for _, w := range mediumRisk {
		m.AddWord(w, 40)
	}
}

// AddWord inserts a keyword with its risk score into the Trie before Build.
func (m *ACMatcher) AddWord(word string, riskScore int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	word = strings.TrimSpace(strings.ToLower(word))
	if word == "" {
		return
	}
	curr := m.root
	for _, r := range word {
		child, ok := curr.children[r]
		if !ok {
			child = newACNode()
			curr.children[r] = child
		}
		curr = child
	}
	curr.patterns = append(curr.patterns, acPattern{
		word:      word,
		riskScore: riskScore,
	})
}

// Build constructs the BFS failure links across the Trie.
func (m *ACMatcher) Build() {
	m.mu.Lock()
	defer m.mu.Unlock()

	queue := make([]*acNode, 0)
	for _, child := range m.root.children {
		child.fail = m.root
		queue = append(queue, child)
	}

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		for r, child := range curr.children {
			f := curr.fail
			for f != nil {
				if next, ok := f.children[r]; ok {
					child.fail = next
					break
				}
				f = f.fail
			}
			if child.fail == nil {
				child.fail = m.root
			}
			if child.fail != nil && len(child.fail.patterns) > 0 {
				child.patterns = append(child.patterns, child.fail.patterns...)
			}
			queue = append(queue, child)
		}
	}
}

// Match scans text against the Aho-Corasick automaton in O(n) time.
func (m *ACMatcher) Match(text string) domainmoderation.MatchResult {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cleanText := strings.ToLower(text)
	curr := m.root
	matchedMap := make(map[string]int)
	maxRisk := 0

	for _, r := range cleanText {
		for curr != nil {
			if next, ok := curr.children[r]; ok {
				curr = next
				break
			}
			if curr == m.root {
				curr = m.root
				break
			}
			curr = curr.fail
		}
		if curr == nil {
			curr = m.root
		}

		if len(curr.patterns) > 0 {
			for _, pat := range curr.patterns {
				matchedMap[pat.word] = pat.riskScore
				if pat.riskScore > maxRisk {
					maxRisk = pat.riskScore
				}
			}
		}
	}

	words := make([]string, 0, len(matchedMap))
	for w := range matchedMap {
		words = append(words, w)
	}

	return domainmoderation.MatchResult{
		MatchedWords: words,
		RiskScore:    maxRisk,
	}
}
