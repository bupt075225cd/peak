package service

import (
	"context"
	"testing"
)

func TestComputeWordFreq(t *testing.T) {
	contents := []string{
		"明月几时有，把酒问青天。", // 有/天 非停用词；"不"等停用词过滤
		"月是故乡明。",
	}
	freq, total := computeWordFreq(contents)
	if total == 0 {
		t.Fatal("total chars should be > 0")
	}
	if len(freq) == 0 {
		t.Fatal("freq should not be empty")
	}
	// 频次降序。
	for i := 1; i < len(freq); i++ {
		if freq[i-1].Count < freq[i].Count {
			t.Fatalf("freq not sorted desc at %d: %+v", i, freq[i-1:i+1])
		}
	}
	// "月"与"明"各出现 2 次（并列最高），停用词"有""不""之""而"不得出现。
	if freq[0].Count != 2 {
		t.Errorf("top word count = %+v, want 2", freq[0])
	}
	counts := map[string]int{}
	for _, w := range freq {
		counts[w.Word] = w.Count
	}
	for _, word := range []string{"月", "明"} {
		if counts[word] != 2 {
			t.Errorf("word %q count = %d, want 2", word, counts[word])
		}
	}
	for _, s := range []string{"有", "不", "之", "而"} {
		for _, w := range freq {
			if w.Word == s {
				t.Errorf("stopword %q should be filtered", s)
			}
		}
	}
}

func TestVolumeStatsOrdering(t *testing.T) {
	// 直接验证固定册数顺序（纯逻辑，不依赖 DB）。
	if len(Volumes) != 6 {
		t.Fatalf("Volumes length = %d, want 6", len(Volumes))
	}
	if Volumes[0] != "七年级上册" || Volumes[5] != "九年级下册" {
		t.Errorf("Volumes order unexpected: %v", Volumes)
	}
	if len(Categories) != 2 {
		t.Errorf("Categories = %v, want 课内/课外", Categories)
	}
}

func TestChat(t *testing.T) {
	s := &Service{}
	resp := s.Chat(context.TODO(), ChatRequest{Question: "《岳阳楼记》的作者是谁？"})
	if resp.Enabled {
		t.Error("chat placeholder should not be enabled")
	}
	if resp.Notice == "" {
		t.Error("empty question should return notice")
	}
	resp = s.Chat(context.TODO(), ChatRequest{})
	if resp.Notice == "" {
		t.Error("chat should return notice for empty question")
	}
}

func TestParseUint(t *testing.T) {
	if v, err := ParseUint("42"); err != nil || v != 42 {
		t.Errorf("ParseUint(42) = %v, %v", v, err)
	}
	if _, err := ParseUint("abc"); err == nil {
		t.Error("ParseUint(abc) should fail")
	}
}
