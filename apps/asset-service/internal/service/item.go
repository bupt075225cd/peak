package service

import (
	"context"
	"strconv"
	"strings"
	"time"

	"peak/apps/asset-service/internal/repository"
	"peak/libs/domain"
	bizerr "peak/libs/errors"
	"peak/libs/security"
)

// ItemView 解密后的条目视图（仅在进程内存与 HTTPS 响应中出现）。
type ItemView struct {
	ID          uint64    `json:"id"`
	Name        string    `json:"name"`
	Category    string    `json:"category"`
	SubType     string    `json:"sub_type"`
	AmountCents int64     `json:"amount_cents"`
	Currency    string    `json:"currency"`
	RiskType    string    `json:"risk_type"`
	MemberID    *uint64   `json:"member_id"`
	MemberName  string    `json:"member_name"`
	Note        string    `json:"note"`
	Version     uint32    `json:"version"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ItemFilter 列表筛选（解密后内存过滤，密文列无法走 SQL LIKE）。
type ItemFilter struct {
	Category string
	Keyword  string
	Offset   int
	Limit    int
}

// ItemListResult 列表结果与汇总。
type ItemListResult struct {
	Items          []ItemView     `json:"items"`
	Total          int            `json:"total"`
	SumCents       int64          `json:"sum_cents"`
	CategoryCounts map[string]int `json:"category_counts"`
}

// CreateItem 新增条目（事务内：写条目 → 重算当月快照 → 追加审计）。
func (s *Service) CreateItem(ctx context.Context, owner, traceID string, in ItemInput) (*ItemView, error) {
	if err := validateItem(&in); err != nil {
		return nil, err
	}
	var view *ItemView
	err := s.repo.Transaction(ctx, func(tx *repository.Repository) error {
		if err := s.ensureConsent(ctx, tx, owner); err != nil {
			return err
		}
		count, err := tx.CountItems(ctx, owner)
		if err != nil {
			return err
		}
		if count >= int64(s.opts.MaxItems) {
			return bizerr.New(bizerr.CodeInvalidArgument, "条目数量已达上限，请先归档或删除历史记录")
		}
		dek, keyVersion, err := s.dekFor(ctx, tx, owner)
		if err != nil {
			return err
		}
		memberName, err := s.resolveMemberName(ctx, tx, owner, dek, in.MemberID)
		if err != nil {
			return err
		}
		rowUID, err := security.NewRowUID()
		if err != nil {
			return err
		}
		now := s.now().UTC()
		row := &domain.LedgerItem{
			OwnerRef: owner, RowUID: rowUID, Category: in.Category,
			Currency: in.Currency, RiskType: in.RiskType, MemberID: in.MemberID,
			KeyVersion: keyVersion, Version: 1, CreatedAt: now, UpdatedAt: now,
		}
		if err := s.fillCiphers(row, dek, in, memberName); err != nil {
			return err
		}
		row.RowMAC = s.keys.RowMAC(s.itemRow(row).Canonical())
		if err := tx.CreateItem(ctx, row); err != nil {
			return err
		}
		if err := s.recomputeSnapshot(ctx, tx, owner, dek, keyVersion); err != nil {
			return err
		}
		if err := tx.AppendAudit(ctx, owner, "item.create", "item", strconv.FormatUint(row.ID, 10), traceID); err != nil {
			return err
		}
		view = &ItemView{
			ID: row.ID, Name: in.Name, Category: row.Category, SubType: in.SubType,
			AmountCents: in.AmountCents, Currency: row.Currency, RiskType: row.RiskType,
			MemberID: row.MemberID, MemberName: memberName, Note: in.Note,
			Version: row.Version, UpdatedAt: row.UpdatedAt,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return view, nil
}

// UpdateItem 更新条目（乐观锁：版本不符返回 409）。
func (s *Service) UpdateItem(ctx context.Context, owner, traceID string, id uint64, in UpdateInput) (*ItemView, error) {
	if err := validateItem(&in.ItemInput); err != nil {
		return nil, err
	}
	var view *ItemView
	err := s.repo.Transaction(ctx, func(tx *repository.Repository) error {
		if err := s.ensureConsent(ctx, tx, owner); err != nil {
			return err
		}
		row, err := tx.GetItem(ctx, owner, id)
		if err != nil {
			return err
		}
		if row == nil {
			return notFound()
		}
		if in.Version != 0 && row.Version != in.Version {
			return bizerr.New(bizerr.CodeConflict, "该记录已被修改，请刷新后重试")
		}
		dek, keyVersion, err := s.dekFor(ctx, tx, owner)
		if err != nil {
			return err
		}
		memberName, err := s.resolveMemberName(ctx, tx, owner, dek, in.MemberID)
		if err != nil {
			return err
		}
		expect := row.Version
		row.Category = in.Category
		row.Currency = in.Currency
		row.RiskType = in.RiskType
		row.MemberID = in.MemberID
		row.KeyVersion = keyVersion
		row.Version = expect + 1
		row.UpdatedAt = s.now().UTC()
		if err := s.fillCiphers(row, dek, in.ItemInput, memberName); err != nil {
			return err
		}
		row.RowMAC = s.keys.RowMAC(s.itemRow(row).Canonical())
		ok, err := tx.UpdateItem(ctx, row, expect)
		if err != nil {
			return err
		}
		if !ok {
			return bizerr.New(bizerr.CodeConflict, "该记录已被修改，请刷新后重试")
		}
		if err := s.recomputeSnapshot(ctx, tx, owner, dek, keyVersion); err != nil {
			return err
		}
		if err := tx.AppendAudit(ctx, owner, "item.update", "item", strconv.FormatUint(row.ID, 10), traceID); err != nil {
			return err
		}
		view = &ItemView{
			ID: row.ID, Name: in.Name, Category: row.Category, SubType: in.SubType,
			AmountCents: in.AmountCents, Currency: row.Currency, RiskType: row.RiskType,
			MemberID: row.MemberID, MemberName: memberName, Note: in.Note,
			Version: row.Version, UpdatedAt: row.UpdatedAt,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return view, nil
}

// DeleteItem 删除条目。
func (s *Service) DeleteItem(ctx context.Context, owner, traceID string, id uint64) error {
	return s.repo.Transaction(ctx, func(tx *repository.Repository) error {
		if err := s.ensureConsent(ctx, tx, owner); err != nil {
			return err
		}
		row, err := tx.GetItem(ctx, owner, id)
		if err != nil {
			return err
		}
		if row == nil {
			return notFound()
		}
		dek, keyVersion, err := s.dekFor(ctx, tx, owner)
		if err != nil {
			return err
		}
		if err := tx.DeleteItem(ctx, row); err != nil {
			return err
		}
		if err := s.recomputeSnapshot(ctx, tx, owner, dek, keyVersion); err != nil {
			return err
		}
		return tx.AppendAudit(ctx, owner, "item.delete", "item", strconv.FormatUint(id, 10), traceID)
	})
}

// ListItems 解密后筛选、排序、分页，并返回合计与分类计数。
func (s *Service) ListItems(ctx context.Context, owner string, f ItemFilter) (*ItemListResult, error) {
	views, err := s.loadItems(ctx, s.repo, owner)
	if err != nil {
		return nil, err
	}
	keyword := strings.ToLower(strings.TrimSpace(f.Keyword))
	counts := map[string]int{}
	filtered := make([]ItemView, 0, len(views))
	var sum int64
	for _, v := range views {
		counts[v.Category]++
		if f.Category != "" && v.Category != f.Category {
			continue
		}
		if keyword != "" && !strings.Contains(strings.ToLower(v.Name+" "+v.Note+" "+v.SubType), keyword) {
			continue
		}
		filtered = append(filtered, v)
		sum += v.AmountCents
	}
	// 已按 updated_at 倒序（loadItems 保证）。
	total := len(filtered)
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > MaxListLimit {
		limit = MaxListLimit
	}
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return &ItemListResult{
		Items: filtered[offset:end], Total: total, SumCents: sum, CategoryCounts: counts,
	}, nil
}

// loadItems 读取并逐行校验 MAC、解密；按更新时间倒序。
func (s *Service) loadItems(ctx context.Context, repo *repository.Repository, owner string) ([]ItemView, error) {
	rows, err := repo.ListItems(ctx, owner)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []ItemView{}, nil
	}
	dek, _, err := s.dekFor(ctx, repo, owner)
	if err != nil {
		return nil, err
	}
	out := make([]ItemView, 0, len(rows))
	for i := range rows {
		view, err := s.openItem(dek, &rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, *view)
	}
	// 稳定排序：更新时间倒序，ID 倒序兜底。
	sortItems(out)
	return out, nil
}

// openItem 校验行 MAC 后解密单个条目。
func (s *Service) openItem(dek []byte, row *domain.LedgerItem) (*ItemView, error) {
	if !s.keys.VerifyRowMAC(s.itemRow(row).Canonical(), row.RowMAC) {
		return nil, bizerr.New(bizerr.CodeInternal, "数据完整性校验失败")
	}
	read := func(cipher []byte, field string) (string, error) {
		plain, err := s.keys.Open(dek, cipher, security.BuildAAD(row.OwnerRef, row.RowUID, field))
		if err != nil {
			return "", bizerr.New(bizerr.CodeInternal, "数据解密失败")
		}
		return string(plain), nil
	}
	name, err := read(row.NameCipher, "name")
	if err != nil {
		return nil, err
	}
	sub, err := read(row.SubCipher, "sub_type")
	if err != nil {
		return nil, err
	}
	amountRaw, err := read(row.AmtCipher, "amount")
	if err != nil {
		return nil, err
	}
	note, err := read(row.NoteCipher, "note")
	if err != nil {
		return nil, err
	}
	member, err := read(row.MemCipher, "member_name")
	if err != nil {
		return nil, err
	}
	amount, err := strconv.ParseInt(amountRaw, 10, 64)
	if err != nil {
		return nil, bizerr.New(bizerr.CodeInternal, "数据格式损坏")
	}
	return &ItemView{
		ID: row.ID, Name: name, Category: row.Category, SubType: sub,
		AmountCents: amount, Currency: row.Currency, RiskType: row.RiskType,
		MemberID: row.MemberID, MemberName: member, Note: note,
		Version: row.Version, UpdatedAt: row.UpdatedAt,
	}, nil
}

// fillCiphers 加密全部敏感字段。
func (s *Service) fillCiphers(row *domain.LedgerItem, dek []byte, in ItemInput, memberName string) error {
	seal := func(plain, field string) ([]byte, error) {
		return s.keys.Seal(dek, []byte(plain), security.BuildAAD(row.OwnerRef, row.RowUID, field))
	}
	var err error
	if row.NameCipher, err = seal(in.Name, "name"); err != nil {
		return err
	}
	if row.SubCipher, err = seal(in.SubType, "sub_type"); err != nil {
		return err
	}
	if row.AmtCipher, err = seal(strconv.FormatInt(in.AmountCents, 10), "amount"); err != nil {
		return err
	}
	if row.NoteCipher, err = seal(in.Note, "note"); err != nil {
		return err
	}
	if row.MemCipher, err = seal(memberName, "member_name"); err != nil {
		return err
	}
	return nil
}

// resolveMemberName 校验成员归属并返回昵称（写入条目时做快照）。
func (s *Service) resolveMemberName(ctx context.Context, repo *repository.Repository, owner string, dek []byte, memberID *uint64) (string, error) {
	if memberID == nil {
		return "", nil
	}
	member, err := repo.GetMember(ctx, owner, *memberID)
	if err != nil {
		return "", err
	}
	if member == nil {
		return "", bizerr.New(bizerr.CodeInvalidArgument, "成员不存在")
	}
	name, err := s.openMember(dek, member)
	if err != nil {
		return "", err
	}
	return name, nil
}

func (s *Service) itemRow(row *domain.LedgerItem) security.ItemRow {
	var memberID uint64
	if row.MemberID != nil {
		memberID = *row.MemberID
	}
	return security.ItemRow{
		OwnerRef: row.OwnerRef, RowUID: row.RowUID, Category: row.Category,
		Currency: row.Currency, RiskType: row.RiskType, MemberID: memberID,
		KeyVersion: row.KeyVersion, Version: row.Version, UpdatedAt: row.UpdatedAt,
		NameCipher: row.NameCipher, SubCipher: row.SubCipher, AmtCipher: row.AmtCipher,
		NoteCipher: row.NoteCipher, MemCipher: row.MemCipher,
	}
}
