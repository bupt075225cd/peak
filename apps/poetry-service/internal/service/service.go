// Package service 古诗文业务逻辑：多维筛选、统计聚合、词频统计（进程内缓存）
// 与 RAG 智能对话占位。
package service

import (
	"context"
	"sort"
	"strconv"
	"sync"
	"unicode"

	"peak/apps/poetry-service/internal/repository"
	"peak/libs/domain"
	"peak/libs/errors"
)

// Volumes 册数固定顺序（七~九年级上/下册）。
var Volumes = []string{
	"七年级上册", "七年级下册", "八年级上册", "八年级下册", "九年级上册", "九年级下册",
}

// Categories 分类取值。
var Categories = []string{"课内", "课外"}

// stopwords 文言高频虚词停用词表：单字词云中占比过高、信息量低。
var stopwords = map[string]bool{
	"之": true, "乎": true, "者": true, "也": true, "矣": true, "焉": true,
	"哉": true, "而": true, "于": true, "以": true, "其": true, "为": true,
	"则": true, "乃": true, "且": true, "夫": true, "盖": true, "所": true,
	"与": true, "及": true, "即": true, "遂": true, "皆": true, "云": true,
	"曰": true, "一": true, "不": true, "人": true, "有": true, "无": true,
	"我": true, "吾": true, "尔": true, "汝": true, "是": true, "此": true,
	"彼": true, "何": true, "安": true, "孰": true, "或": true, "兮": true,
	"耳": true, "耶": true, "邪": true, "噫": true, "嗟": true, "惟": true,
	"诸": true, "若": true, "如": true, "因": true, "故": true, "然": true,
}

// WordStat 单个字词的频次。
type WordStat struct {
	Word  string `json:"word"`
	Count int    `json:"count"`
}

// Service 古诗文业务服务。
type Service struct {
	repo repository.PoemRepository

	// 词频统计缓存：语料固定（启动导入），全量计算一次即可。
	wordMu     sync.Mutex
	wordCached bool
	wordFreq   []WordStat
	wordTotal  int
}

// New 创建服务实例。
func New(repo repository.PoemRepository) *Service {
	return &Service{repo: repo}
}

// ListPoems 分页查询篇目。
func (s *Service) ListPoems(ctx context.Context, f repository.Filter) ([]domain.Poem, int64, error) {
	return s.repo.List(ctx, f)
}

// GetPoem 按 ID 查询篇目；不存在返回业务 NotFound 错误。
func (s *Service) GetPoem(ctx context.Context, id uint64) (*domain.Poem, error) {
	p, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, errors.New(errors.CodeNotFound, "篇目不存在")
	}
	return p, nil
}

// RelatedPoems 相关篇目推荐（同作者优先，其次同朝代）。
func (s *Service) RelatedPoems(ctx context.Context, poem *domain.Poem, limit int) ([]domain.Poem, error) {
	return s.repo.Related(ctx, poem, limit)
}

// DynastyStat 朝代分布统计项（含时间轴排序）。
type DynastyStat struct {
	Dynasty string `json:"dynasty"`
	Order   int    `json:"order"`
	Count   int64  `json:"count"`
}

// VolumeStat 册数分布统计项（课内/课外拆分）。
type VolumeStat struct {
	Volume  string `json:"volume"`
	Inclass int64  `json:"inclass"`
	Outline int64  `json:"outline"`
	Other   int64  `json:"other"` // 无分类标注（旧版未收录篇目）
}

// DynastyStats 全部朝代分布（固定顺序，无篇目的朝代计数为 0）。
func (s *Service) DynastyStats(ctx context.Context) ([]DynastyStat, error) {
	counts, err := s.repo.CountByDynasty(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]DynastyStat, 0, len(domain.PoemDynasties))
	for i, d := range domain.PoemDynasties {
		out = append(out, DynastyStat{Dynasty: d, Order: i, Count: counts[d]})
	}
	return out, nil
}

// VolumeStats 按册数统计（课内/课外拆分），标准册数按固定顺序，
// 非标准册数（如旧版未收录标注）按字典序追加在末尾。
func (s *Service) VolumeStats(ctx context.Context) ([]VolumeStat, error) {
	rows, err := s.repo.CountByVolume(ctx)
	if err != nil {
		return nil, err
	}
	byVolume := map[string]*VolumeStat{}
	for _, vol := range Volumes {
		byVolume[vol] = &VolumeStat{Volume: vol}
	}
	others := map[string]*VolumeStat{}
	for _, r := range rows {
		st := byVolume[r.Volume]
		if st == nil {
			st = others[r.Volume]
			if st == nil {
				st = &VolumeStat{Volume: r.Volume}
				others[r.Volume] = st
			}
		}
		switch r.Category {
		case "课内":
			st.Inclass += r.Count
		case "课外":
			st.Outline += r.Count
		default:
			st.Other += r.Count
		}
	}
	out := make([]VolumeStat, 0, len(Volumes)+len(others))
	for _, vol := range Volumes {
		out = append(out, *byVolume[vol])
	}
	extra := make([]string, 0, len(others))
	for v := range others {
		extra = append(extra, v)
	}
	sort.Strings(extra)
	for _, v := range extra {
		out = append(out, *others[v])
	}
	return out, nil
}

// Authors 作者作品数排行。
func (s *Service) Authors(ctx context.Context, top int) ([]repository.AuthorCount, error) {
	return s.repo.AuthorRanking(ctx, top)
}

// WordFreq 全文高频字词统计（汉字单字，过滤虚词停用词与标点），
// 结果进程内缓存。top ≤ 0 返回全部（按频次降序），第二返回值为统计总字数。
func (s *Service) WordFreq(ctx context.Context, top int) ([]WordStat, int, error) {
	s.wordMu.Lock()
	defer s.wordMu.Unlock()
	if !s.wordCached {
		contents, err := s.repo.AllContents(ctx)
		if err != nil {
			return nil, 0, err
		}
		s.wordFreq, s.wordTotal = computeWordFreq(contents)
		s.wordCached = true
	}
	freq := s.wordFreq
	if top > 0 && top < len(freq) {
		freq = freq[:top]
	}
	return freq, s.wordTotal, nil
}

// computeWordFreq 统计全部正文的高频汉字（过滤停用虚词）。
func computeWordFreq(contents []string) ([]WordStat, int) {
	counter := map[string]int{}
	total := 0
	for _, c := range contents {
		for _, r := range c {
			if !unicode.Is(unicode.Han, r) {
				continue
			}
			total++
			w := string(r)
			if stopwords[w] {
				continue
			}
			counter[w]++
		}
	}
	out := make([]WordStat, 0, len(counter))
	for w, n := range counter {
		out = append(out, WordStat{Word: w, Count: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Word < out[j].Word
	})
	return out, total
}

// ChatRequest 智能对话请求体（RAG 二阶段实现，当前仅契约占位）。
type ChatRequest struct {
	Question string `json:"question"`
}

// ChatResponse 智能对话响应体。
type ChatResponse struct {
	Enabled bool   `json:"enabled"`          // 功能是否已启用（当前恒为 false）
	Answer  string `json:"answer,omitempty"` // 启用后返回的回答
	Notice  string `json:"notice"`           // 未启用时的提示文案
}

// Chat RAG 智能对话占位：二阶段将基于语料向量检索 + LLM 生成回答，
// 当前返回明确的"未启用"响应，接口契约保持稳定。
func (s *Service) Chat(_ context.Context, req ChatRequest) ChatResponse {
	if req.Question == "" {
		return ChatResponse{Enabled: false, Notice: "请输入想了解的古诗文问题"}
	}
	return ChatResponse{
		Enabled: false,
		Notice:  "「古诗文智能问答」即将上线，届时可基于全部篇目语料为你答疑解惑，敬请期待。",
	}
}

// ParseUint 解析十进制 ID 字符串。
func ParseUint(s string) (uint64, error) {
	return strconv.ParseUint(s, 10, 64)
}
