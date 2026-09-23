package domain

// MistakeQuery 错题列表查询条件：分页 + 过滤。
//
// 放在 domain 层是为了让 handler 与 repository 共用同一套查询结构，避免上层直接依赖仓储实现。
type MistakeQuery struct {
	// UserID 仅查询该用户的错题（必填）。
	UserID uint64
	// Keyword 关键词，空白分隔多个词；每个词都必须命中 题干/知识点/题型/来源/学科 之一。
	Keyword string
	// Subject 学科精确匹配，空表示不限。
	Subject string
	// Source 来源精确匹配（优先题目来源，为空再取错题记录来源），空表示不限。
	Source string
	// Offset/Limit 分页参数。
	Offset int
	Limit  int
}

// MistakeListResult 错题列表查询结果。
type MistakeListResult struct {
	// Items 当前页的错题。
	Items []Mistake
	// Total 符合筛选条件的错题总数，用于分页与"共 N 道"展示。
	Total int64
	// SubjectCounts 学科分布计数。仅按关键词统计（不含学科/来源过滤），
	// 否则切换学科筛选后其它分支的计数会变成 0。
	SubjectCounts map[string]int64
	// SourceCounts 来源分布计数，规则同上；来源为空的历史数据不包含在内。
	SourceCounts map[string]int64
}
