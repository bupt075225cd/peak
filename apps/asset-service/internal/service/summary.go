package service

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"peak/apps/asset-service/internal/repository"
	"peak/libs/domain"
	bizerr "peak/libs/errors"
	"peak/libs/security"
)

// SummaryChild 子类聚合行。
type SummaryChild struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	AmountCents int64  `json:"amount_cents"`
	Count       int    `json:"count"`
	IsFixed     bool   `json:"is_fixed"`
}

// SummaryGroup 分类聚合行。
type SummaryGroup struct {
	Category    string         `json:"category"`
	Label       string         `json:"label"`
	Kind        string         `json:"kind"` // asset / liability
	Color       string         `json:"color"`
	AmountCents int64          `json:"amount_cents"`
	Count       int            `json:"count"`
	Children    []SummaryChild `json:"children"`
}

// Summary 总览聚合结果（解密后在内存完成，SQL 不接触密文）。
type Summary struct {
	Groups              []SummaryGroup `json:"groups"`
	TotalAssetCents     int64          `json:"total_asset_cents"`
	TotalLiabilityCents int64          `json:"total_liability_cents"`
	NetAssetCents       int64          `json:"net_asset_cents"`
	ItemCount           int            `json:"item_count"`
	UpdatedAt           time.Time      `json:"updated_at"`
	HasForeign          bool           `json:"has_foreign"`
	ConsentRequired     bool           `json:"consent_required"`
}

// SnapshotBreakdown 快照明细行。
type SnapshotBreakdown struct {
	Category    string `json:"category"`
	Label       string `json:"label"`
	AmountCents int64  `json:"amount_cents"`
	Count       int    `json:"count"`
}

// SnapshotPayload 快照明文载荷（整体加密后落库）。
type SnapshotPayload struct {
	TotalAssetCents     int64               `json:"total_asset_cents"`
	TotalLiabilityCents int64               `json:"total_liability_cents"`
	NetAssetCents       int64               `json:"net_asset_cents"`
	Breakdown           []SnapshotBreakdown `json:"breakdown"`
}

// SnapshotView 快照视图。
type SnapshotView struct {
	Period    string          `json:"period"`
	Payload   SnapshotPayload `json:"payload"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// Summary 返回分类/子类聚合。
//
// memberID 非空时只聚合该成员名下的条目，用于资产组成页按成员查看；
// 过滤在解密后的视图上完成，避免对密文列做 SQL 条件。
func (s *Service) Summary(ctx context.Context, owner string, memberID *uint64) (*Summary, error) {
	return s.SummaryScoped(ctx, owner, memberID, false)
}

// SummaryScoped 按成员范围聚合：memberID 非空看该成员，
// onlySelf 为 true 时只看未归属任何成员的条目（总览里的「我」）。
func (s *Service) SummaryScoped(ctx context.Context, owner string, memberID *uint64, onlySelf bool) (*Summary, error) {
	if err := s.ensureConsent(ctx, s.repo, owner); err != nil {
		return nil, err
	}
	views, err := s.loadItems(ctx, s.repo, owner)
	if err != nil {
		return nil, err
	}
	if memberID != nil {
		filtered := make([]ItemView, 0, len(views))
		for _, view := range views {
			if view.MemberID != nil && *view.MemberID == *memberID {
				filtered = append(filtered, view)
			}
		}
		views = filtered
	} else if onlySelf {
		filtered := make([]ItemView, 0, len(views))
		for _, view := range views {
			if view.MemberID == nil {
				filtered = append(filtered, view)
			}
		}
		views = filtered
	}
	return buildSummary(views), nil
}

// buildSummary 按分类 → 子类聚合，保持元数据中的固定顺序。
func buildSummary(items []ItemView) *Summary {
	byCategory := map[string]*SummaryGroup{}
	childIndex := map[string]map[string]int{}
	var (
		totalAsset, totalLiability int64
		updatedAt                  time.Time
		hasForeign                 bool
	)
	for _, c := range domain.LedgerCategories() {
		byCategory[c.Key] = &SummaryGroup{
			Category: c.Key, Label: c.Label, Kind: c.Kind, Color: c.Color, Children: []SummaryChild{},
		}
		childIndex[c.Key] = map[string]int{}
	}
	for _, item := range items {
		group, ok := byCategory[item.Category]
		if !ok {
			continue
		}
		group.AmountCents += item.AmountCents
		group.Count++
		if domain.LedgerIsLiability(item.Category) {
			totalLiability += item.AmountCents
		} else {
			totalAsset += item.AmountCents
		}
		if item.UpdatedAt.After(updatedAt) {
			updatedAt = item.UpdatedAt
		}
		if item.Currency != domain.LedgerCurrencyCNY {
			hasForeign = true
		}
		idx, seen := childIndex[item.Category][item.SubType]
		if !seen {
			child := SummaryChild{
				Key: item.SubType, Label: domain.LedgerSubTypeLabel(item.SubType),
				IsFixed: item.Category == domain.LedgerCategoryFixed,
			}
			group.Children = append(group.Children, child)
			idx = len(group.Children) - 1
			childIndex[item.Category][item.SubType] = idx
		}
		group.Children[idx].AmountCents += item.AmountCents
		group.Children[idx].Count++
	}
	groups := make([]SummaryGroup, 0, len(byCategory))
	for _, c := range domain.LedgerCategories() {
		group := byCategory[c.Key]
		sort.SliceStable(group.Children, func(i, j int) bool {
			return group.Children[i].AmountCents > group.Children[j].AmountCents
		})
		groups = append(groups, *group)
	}
	return &Summary{
		Groups: groups, TotalAssetCents: totalAsset, TotalLiabilityCents: totalLiability,
		NetAssetCents: totalAsset - totalLiability, ItemCount: len(items),
		UpdatedAt: updatedAt, HasForeign: hasForeign,
	}
}

// recomputeSnapshot 重算并覆盖当月快照（与条目写入同事务）。
func (s *Service) recomputeSnapshot(ctx context.Context, repo *repository.Repository, owner string, dek []byte, keyVersion int) error {
	views, err := s.loadItems(ctx, repo, owner)
	if err != nil {
		return err
	}
	summary := buildSummary(views)
	payload := SnapshotPayload{
		TotalAssetCents: summary.TotalAssetCents, TotalLiabilityCents: summary.TotalLiabilityCents,
		NetAssetCents: summary.NetAssetCents, Breakdown: []SnapshotBreakdown{},
	}
	for _, g := range summary.Groups {
		if g.Count == 0 {
			continue
		}
		payload.Breakdown = append(payload.Breakdown, SnapshotBreakdown{
			Category: g.Category, Label: g.Label, AmountCents: g.AmountCents, Count: g.Count,
		})
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	now := s.now().UTC()
	period := domain.LedgerPeriod(now)
	sealed, err := s.keys.Seal(dek, raw, security.BuildAAD(owner, period, "snapshot"))
	if err != nil {
		return err
	}
	return repo.UpsertSnapshot(ctx, &domain.LedgerSnapshot{
		OwnerRef: owner, Period: period, PayloadCipher: sealed,
		KeyVersion: keyVersion, UpdatedAt: now,
	})
}

// Snapshots 返回指定年份的月度快照（解密后）。
func (s *Service) Snapshots(ctx context.Context, owner, year string) ([]SnapshotView, error) {
	if year == "" {
		year = s.now().UTC().Format("2006")
	}
	if len(year) != 4 {
		return nil, bizerr.New(bizerr.CodeInvalidArgument, "年份格式应为 YYYY")
	}
	if err := s.ensureConsent(ctx, s.repo, owner); err != nil {
		return nil, err
	}
	rows, err := s.repo.ListSnapshots(ctx, owner, year+"-")
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []SnapshotView{}, nil
	}
	dek, _, err := s.dekFor(ctx, s.repo, owner)
	if err != nil {
		return nil, err
	}
	out := make([]SnapshotView, 0, len(rows))
	for i := range rows {
		row := &rows[i]
		plain, err := s.keys.Open(dek, row.PayloadCipher, security.BuildAAD(owner, row.Period, "snapshot"))
		if err != nil {
			return nil, bizerr.New(bizerr.CodeInternal, "快照解密失败")
		}
		var payload SnapshotPayload
		if err := json.Unmarshal(plain, &payload); err != nil {
			return nil, bizerr.New(bizerr.CodeInternal, "快照数据格式损坏")
		}
		out = append(out, SnapshotView{Period: row.Period, Payload: payload, UpdatedAt: row.UpdatedAt})
	}
	return out, nil
}

// sortItems 更新时间倒序，ID 倒序兜底。
func sortItems(items []ItemView) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].ID > items[j].ID
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
}
