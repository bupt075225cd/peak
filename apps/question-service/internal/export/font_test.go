package export

import (
	"testing"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
)

func TestFontProviderParsesEmbeddedFont(t *testing.T) {
	if _, err := NewFontProvider(""); err != nil {
		t.Fatalf("NewFontProvider: %v", err)
	}
}

func TestEmbeddedFontHasChineseGlyphs(t *testing.T) {
	f, err := opentype.Parse(embeddedFont)
	if err != nil {
		t.Fatalf("parse embedded font: %v", err)
	}

	var buf sfnt.Buffer
	for _, r := range []rune{'错', '题', '本', '数', '学', '已', '知', '求', '解'} {
		idx, err := f.GlyphIndex(&buf, r)
		if err != nil {
			t.Fatalf("glyph index %q: %v", r, err)
		}
		if idx == 0 {
			t.Fatalf("embedded font missing glyph for %q", r)
		}
	}
}

func TestFontProviderCachesFaceBySize(t *testing.T) {
	p, err := NewFontProvider("")
	if err != nil {
		t.Fatalf("NewFontProvider: %v", err)
	}

	a, err := p.Face(14)
	if err != nil {
		t.Fatalf("Face(14): %v", err)
	}
	b, err := p.Face(14)
	if err != nil {
		t.Fatalf("Face(14) again: %v", err)
	}
	if a != b {
		t.Fatal("expected face reused for same size")
	}

	c, err := p.Face(18)
	if err != nil {
		t.Fatalf("Face(18): %v", err)
	}
	if a == c {
		t.Fatal("expected distinct face for different size")
	}
}

func TestFontProviderMeasuresChineseText(t *testing.T) {
	p, err := NewFontProvider("")
	if err != nil {
		t.Fatalf("NewFontProvider: %v", err)
	}
	face, err := p.Face(12)
	if err != nil {
		t.Fatalf("Face: %v", err)
	}
	if w := font.MeasureString(face, "错题本"); w <= 0 {
		t.Fatalf("expected positive width for chinese text, got %v", w)
	}
}

func TestFontProviderMissingFile(t *testing.T) {
	if _, err := NewFontProvider("/nonexistent/peak-font.otf"); err == nil {
		t.Fatal("expected error for missing font file")
	}
}
