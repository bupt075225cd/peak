package importer

import (
	"strings"
	"testing"

	"peak/libs/domain"
)

// testCorso 最小语料样例：覆盖元信息解析、缺分类段、多朝代与段落聚合。
const testCorpus = `# 初中教材古文与古诗词汇编

> 头部说明块，应被忽略。

## 目录

- **先秦**
- **唐**

## 【先秦】

### 关雎

**作者**：《诗经·周南》　佚名｜ **册数**：八年级下册｜ **分类**：课内

关关雎鸠，在河之洲。窈窕淑女，君子好逑。

参差荇菜，左右流之。

---

### 式微

**作者**：《诗经·邶风》　佚名｜ **册数**：八年级下册｜ **分类**：课外

式微，式微，胡不归？

## 【唐】

### 望岳

**作者**：杜甫｜ **册数**：七年级下册｜ **分类**：课内

岱宗夫如何？齐鲁青未了。

---

### 旧版篇目

**作者**：司马迁（节选）〔非统编版〕｜ **册数**：（统编版未收录，旧人教版九年级下册）

其文缺分类段，正文参读。
`

func TestParse(t *testing.T) {
	poems, err := Parse([]byte(testCorpus))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(poems) != 4 {
		t.Fatalf("Parse() got %d poems, want 4: %+v", len(poems), poems)
	}

	byTitle := map[string]domain.Poem{}
	for _, p := range poems {
		byTitle[p.Title] = p
	}

	gj := byTitle["关雎"]
	if gj.Dynasty != "先秦" || gj.DynastyOrder != 0 {
		t.Errorf("关雎 dynasty = %q(order %d), want 先秦(0)", gj.Dynasty, gj.DynastyOrder)
	}
	if gj.Author != "《诗经·周南》　佚名" || gj.Volume != "八年级下册" || gj.Category != "课内" {
		t.Errorf("关雎 meta = %q/%q/%q", gj.Author, gj.Volume, gj.Category)
	}
	if !strings.Contains(gj.Content, "关关雎鸠") || !strings.Contains(gj.Content, "\n") {
		t.Errorf("关雎 content should join paragraphs with newline, got %q", gj.Content)
	}
	if gj.WordCount == 0 {
		t.Errorf("关雎 word count should be > 0")
	}

	// 跨朝代重名不冲突，旧版篇目分类为空。
	old := byTitle["旧版篇目"]
	if old.Category != "" || old.Dynasty != "唐" {
		t.Errorf("旧版篇目 = %+v", old)
	}

	// 式微之后无 --- 分隔，应由下一个标题收尾，正文不得串篇。
	sw := byTitle["式微"]
	if strings.Contains(sw.Content, "岱宗") {
		t.Errorf("式微 content should not leak next section, got %q", sw.Content)
	}
}

func TestParseEmpty(t *testing.T) {
	if _, err := Parse([]byte("")); err == nil {
		t.Fatal("Parse(empty) should return error")
	}
}

func TestParseMetaMissingAuthor(t *testing.T) {
	if _, _, _, err := parseMeta("**册数**：八年级上册｜ **分类**：课内"); err == nil {
		t.Fatal("parseMeta without author should fail")
	}
}

func TestPoemDynastyOrder(t *testing.T) {
	if got := domain.PoemDynastyOrder("唐"); got != 5 {
		t.Errorf("PoemDynastyOrder(唐) = %d, want 5", got)
	}
	if got := domain.PoemDynastyOrder("不存在"); got != len(domain.PoemDynasties) {
		t.Errorf("unknown dynasty should return len(Dynasties)")
	}
}

// TestLoad 真实语料端到端解析：124 篇、10 个朝代。
func TestLoad(t *testing.T) {
	poems, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(poems) != 124 {
		t.Fatalf("Load() got %d poems, want 124", len(poems))
	}
	dynasties := map[string]int{}
	for _, p := range poems {
		dynasties[p.Dynasty]++
		if p.Title == "" || p.Author == "" || p.Content == "" {
			t.Errorf("poem %q has empty field: %+v", p.Title, p)
		}
	}
	if len(dynasties) != 10 {
		t.Errorf("Load() covers %d dynasties, want 10: %v", len(dynasties), dynasties)
	}
}
