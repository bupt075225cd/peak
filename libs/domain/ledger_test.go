package domain

import (
	"testing"
	"time"
)

func TestLedgerCategoriesTaxonomy(t *testing.T) {
	cats := LedgerCategories()
	if len(cats) != 5 {
		t.Fatalf("categories = %d, want 5", len(cats))
	}

	seenCat := map[string]bool{}
	seenSub := map[string]string{}
	for _, c := range cats {
		if c.Key == "" || c.Label == "" || c.Color == "" {
			t.Fatalf("category meta incomplete: %+v", c)
		}
		if seenCat[c.Key] {
			t.Fatalf("duplicate category key %q", c.Key)
		}
		seenCat[c.Key] = true
		if c.Kind != "asset" && c.Kind != "liability" {
			t.Fatalf("category %q kind = %q", c.Key, c.Kind)
		}
		if len(c.SubTypes) == 0 {
			t.Fatalf("category %q has no sub types", c.Key)
		}
		for _, s := range c.SubTypes {
			if s.Key == "" || s.Label == "" || s.Icon == "" {
				t.Fatalf("sub type meta incomplete: %+v", s)
			}
			// 前端用子类 key 做唯一标识，跨分类重名会串味。
			if owner, ok := seenSub[s.Key]; ok {
				t.Fatalf("duplicate sub type %q in %q and %q", s.Key, owner, c.Key)
			}
			seenSub[s.Key] = c.Key
			if c.Key == LedgerCategoryFixed && !s.IsFixed {
				t.Fatalf("fixed sub type %q must be marked is_fixed", s.Key)
			}
		}
	}

	if _, ok := seenCat[LedgerCategoryLiability]; !ok {
		t.Fatal("liability category missing")
	}
}

func TestLedgerRiskAndCurrencyMeta(t *testing.T) {
	risks := LedgerRiskTypes()
	if len(risks) != 4 {
		t.Fatalf("risk types = %d, want 4", len(risks))
	}
	riskKeys := map[string]bool{}
	for _, r := range risks {
		riskKeys[r.Key] = true
	}
	for _, want := range []string{LedgerRiskCash, LedgerRiskStable, LedgerRiskLong, LedgerRiskNone} {
		if !riskKeys[want] {
			t.Fatalf("risk type %q missing", want)
		}
	}

	currencies := LedgerCurrencies()
	if len(currencies) != 5 {
		t.Fatalf("currencies = %d, want 5", len(currencies))
	}
	for _, c := range currencies {
		if c.Key == "" || c.Label == "" || c.Symbol == "" {
			t.Fatalf("currency meta incomplete: %+v", c)
		}
	}
}

func TestLedgerLabelLookups(t *testing.T) {
	if got := LedgerCategoryLabel(LedgerCategoryFixed); got != "固定资产" {
		t.Fatalf("category label = %q", got)
	}
	// 未知 key 原样返回，避免前端出现空标签。
	if got := LedgerCategoryLabel("unknown"); got != "unknown" {
		t.Fatalf("unknown category label = %q", got)
	}
	if got := LedgerSubTypeLabel("house_self"); got != "房产（自住）" {
		t.Fatalf("sub type label = %q", got)
	}
	if got := LedgerSubTypeLabel("unknown"); got != "unknown" {
		t.Fatalf("unknown sub type label = %q", got)
	}
}

func TestLedgerSubTypeBelongsTo(t *testing.T) {
	if !LedgerSubTypeBelongsTo(LedgerCategoryFixed, "house_self") {
		t.Fatal("house_self should belong to fixed")
	}
	if LedgerSubTypeBelongsTo(LedgerCategoryFixed, "fund") {
		t.Fatal("fund should not belong to fixed")
	}
	if LedgerSubTypeBelongsTo("unknown", "house_self") {
		t.Fatal("unknown category should match nothing")
	}
	if LedgerSubTypeBelongsTo(LedgerCategoryFixed, "unknown") {
		t.Fatal("unknown sub type should not match")
	}
}

func TestLedgerIsLiabilityAndPeriod(t *testing.T) {
	if !LedgerIsLiability(LedgerCategoryLiability) {
		t.Fatal("liability category should be reported as liability")
	}
	if LedgerIsLiability(LedgerCategoryLiquid) {
		t.Fatal("liquid category is not a liability")
	}

	if got := LedgerPeriod(time.Date(2026, 10, 24, 8, 30, 0, 0, time.UTC)); got != "2026-10" {
		t.Fatalf("period = %q", got)
	}
	// 月初/月末都落在同一周期，且补零。
	if got := LedgerPeriod(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)); got != "2026-01" {
		t.Fatalf("period = %q", got)
	}
}
