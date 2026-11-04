package moderation

import (
	"fmt"
	"testing"
)

func TestACMatcher_Basic(t *testing.T) {
	matcher := NewACMatcher()

	// Clean text
	res := matcher.Match("今天天气真好，去公园看小猫咪玩耍")
	if res.RiskScore != 0 || len(res.MatchedWords) != 0 {
		t.Fatalf("expected 0 risk for clean text, got %+v", res)
	}

	// High risk hit
	res2 := matcher.Match("最新线上赌博平台上线，千万彩金等你拿")
	if res2.RiskScore <= 60 || len(res2.MatchedWords) == 0 {
		t.Fatalf("expected high risk > 60, got %+v", res2)
	}
	found := false
	for _, w := range res2.MatchedWords {
		if w == "赌博" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected '赌博' in matched words, got %v", res2.MatchedWords)
	}

	// Medium risk hit
	res3 := matcher.Match("学生党在家兼职刷单，轻松日入过万")
	if res3.RiskScore == 0 || res3.RiskScore > 60 {
		t.Fatalf("expected medium risk (0, 60], got %d", res3.RiskScore)
	}
	if len(res3.MatchedWords) < 2 {
		t.Fatalf("expected at least 2 matched words for '兼职刷单' and '日入过万', got %v", res3.MatchedWords)
	}
}

func TestACMatcher_CustomWordsAndOverlaps(t *testing.T) {
	m := &ACMatcher{
		root: newACNode(),
	}
	m.AddWord("he", 20)
	m.AddWord("she", 30)
	m.AddWord("his", 10)
	m.AddWord("hers", 40)
	m.Build()

	res := m.Match("ushers")
	// "she", "he", "hers" all match
	if len(res.MatchedWords) < 3 {
		t.Fatalf("expected at least 3 matches in 'ushers', got %v", res.MatchedWords)
	}
	if res.RiskScore != 40 {
		t.Fatalf("expected max risk score 40, got %d", res.RiskScore)
	}
}

func BenchmarkACMatcher_Match(b *testing.B) {
	matcher := NewACMatcher()
	// Add 5000 synthetic patterns
	for i := 0; i < 5000; i++ {
		matcher.AddWord(fmt.Sprintf("sensitive_word_%d", i), 50)
	}
	matcher.Build()

	sampleText := "这是一个测试文本，里面包含一些正常内容，比如科技、数码、美食与旅行，偶尔夹带一个 sensitive_word_1234 和 兼职刷单。"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = matcher.Match(sampleText)
	}
}
