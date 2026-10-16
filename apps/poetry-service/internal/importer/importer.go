// Package importer 解析《初中教材古文与古诗词汇编》Markdown 语料，
// 并在服务启动时幂等导入数据库（go:embed 内嵌，无外部文件依赖）。
package importer

import (
	"embed"
	"fmt"
	"strings"
	"unicode"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"peak/libs/domain"
)

//go:embed data/poems.md
var corpusFS embed.FS

// corpusPath 语料文件在 embed 文件系统中的路径。
const corpusPath = "data/poems.md"

// Parse 解析 Markdown 语料为篇目列表。
//
// 语料结构：
//
//	## 【先秦】        → 朝代章节
//	### 关雎           → 篇名（新篇目开始）
//	**作者**：...｜ **册数**：...｜ **分类**：课内   → 元信息行
//	正文段落...        → 直至分隔线 --- 或下一个标题
func Parse(src []byte) ([]domain.Poem, error) {
	text := strings.ReplaceAll(string(src), "\r\n", "\n")
	lines := strings.Split(text, "\n")

	var poems []domain.Poem
	var cur *domain.Poem
	dynasty := ""

	flush := func() {
		if cur != nil {
			cur.Content = strings.TrimSpace(cur.Content)
			cur.WordCount = countCJK(cur.Content)
			if cur.Title == "" {
				return // 无篇名的段落块丢弃
			}
			poems = append(poems, *cur)
		}
	}

	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "## 【") && strings.HasSuffix(line, "】"):
			flush()
			cur = nil
			dynasty = strings.TrimSuffix(strings.TrimPrefix(line, "## 【"), "】")
		case strings.HasPrefix(line, "### "):
			flush()
			title := strings.TrimSpace(strings.TrimPrefix(line, "### "))
			cur = &domain.Poem{Title: title, Dynasty: dynasty, DynastyOrder: domain.PoemDynastyOrder(dynasty)}
		case cur != nil && strings.HasPrefix(line, "**作者**"):
			author, volume, category, err := parseMeta(line)
			if err != nil {
				return nil, fmt.Errorf("parse meta of %q: %w", cur.Title, err)
			}
			cur.Author = author
			cur.Volume = volume
			cur.Category = category
		case cur != nil && strings.HasPrefix(line, "## "):
			// 其他二级标题（如"## 目录"）：结束当前篇目
			flush()
			cur = nil
		case cur != nil && strings.TrimSpace(line) == "---":
		default:
			if cur == nil {
				continue // 头部说明块、目录条目等非篇目内容
			}
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			if cur.Content != "" {
				cur.Content += "\n"
			}
			cur.Content += trimmed
		}
	}
	flush()

	if len(poems) == 0 {
		return nil, fmt.Errorf("no poems parsed from corpus")
	}
	return poems, nil
}

// parseMeta 解析元信息行：以全角竖线 ｜ 分段，各段形如 **键**：值。
// 部分篇目（旧人教版未收录）缺少分类段，缺失字段返回空串。
func parseMeta(line string) (author, volume, category string, err error) {
	for _, part := range strings.Split(line, "｜") {
		part = strings.TrimSpace(part)
		switch {
		case strings.HasPrefix(part, "**作者**"):
			author = metaValue(part, "**作者**")
		case strings.HasPrefix(part, "**册数**"):
			volume = metaValue(part, "**册数**")
		case strings.HasPrefix(part, "**分类**"):
			category = metaValue(part, "**分类**")
		}
	}
	if author == "" {
		return "", "", "", fmt.Errorf("missing author field")
	}
	return author, volume, category, nil
}

// metaValue 提取 "**键**：值" 中的值（兼容全角/半角冒号与粗体标记两侧空格）。
func metaValue(part, key string) string {
	v := strings.TrimPrefix(strings.TrimSpace(part), key)
	v = strings.TrimSpace(strings.TrimPrefix(v, "："))
	v = strings.TrimSpace(strings.TrimPrefix(v, ":"))
	return strings.TrimSpace(v)
}

// countCJK 统计文本中的汉字数（忽略标点、空白与拉丁字符）。
func countCJK(s string) int {
	n := 0
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			n++
		}
	}
	return n
}

// Load 读取内嵌语料并解析为篇目列表。
func Load() ([]domain.Poem, error) {
	src, err := corpusFS.ReadFile(corpusPath)
	if err != nil {
		return nil, fmt.Errorf("read embedded corpus: %w", err)
	}
	return Parse(src)
}

// Seed 将语料幂等导入数据库：按 (title, dynasty) 匹配已存在篇目则更新内容，
// 否则新建。返回导入的篇目总数与本次更新/新增的数量。
func Seed(db *gorm.DB) (total, changed int, err error) {
	poems, err := Load()
	if err != nil {
		return 0, 0, err
	}

	type key struct{ title, dynasty string }
	existing := map[key]uint64{}
	var rows []domain.Poem
	if err := db.Select("id", "title", "dynasty").Find(&rows).Error; err != nil {
		return 0, 0, err
	}
	for _, r := range rows {
		existing[key{r.Title, r.Dynasty}] = r.ID
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		for i := range poems {
			p := poems[i]
			if id, ok := existing[key{p.Title, p.Dynasty}]; ok {
				res := tx.Model(&domain.Poem{ID: id}).Updates(map[string]any{
					"author":        p.Author,
					"volume":        p.Volume,
					"category":      p.Category,
					"content":       p.Content,
					"word_count":    p.WordCount,
					"dynasty_order": p.DynastyOrder,
				})
				if res.Error != nil {
					return res.Error
				}
				if res.RowsAffected > 0 {
					changed++
				}
				total++
				continue
			}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&p).Error; err != nil {
				return err
			}
			total++
			changed++
		}
		return nil
	})
	return total, changed, err
}
