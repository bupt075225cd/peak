package service

import (
	"context"
	"strconv"
	"strings"

	"peak/apps/asset-service/internal/repository"
	"peak/libs/domain"
	bizerr "peak/libs/errors"
	"peak/libs/security"
)

// MemberView 成员标签视图。
type MemberView struct {
	ID        uint64 `json:"id"`
	Name      string `json:"name"`
	SortOrder int    `json:"sort_order"`
	// Visible 是否在家庭资产记账的成员入口中显示。
	Visible bool `json:"visible"`
}

// MemberInput 成员更新入参：字段为 nil 表示不修改。
type MemberInput struct {
	Name    *string `json:"name"`
	Visible *bool   `json:"visible"`
}

// ListMembers 列出成员（解密后）。
func (s *Service) ListMembers(ctx context.Context, owner string) ([]MemberView, error) {
	if err := s.ensureConsent(ctx, s.repo, owner); err != nil {
		return nil, err
	}
	rows, err := s.repo.ListMembers(ctx, owner)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []MemberView{}, nil
	}
	dek, _, err := s.dekFor(ctx, s.repo, owner)
	if err != nil {
		return nil, err
	}
	out := make([]MemberView, 0, len(rows))
	for i := range rows {
		name, err := s.openMember(dek, &rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, MemberView{
			ID: rows[i].ID, Name: name, SortOrder: rows[i].SortOrder, Visible: !rows[i].Hidden,
		})
	}
	return out, nil
}

// CreateMember 新增成员标签（默认在记账中显示）。
func (s *Service) CreateMember(ctx context.Context, owner, traceID, name string) (*MemberView, error) {
	return s.CreateMemberVisible(ctx, owner, traceID, name, true)
}

// CreateMemberVisible 新增成员标签，并指定是否在记账中显示。
func (s *Service) CreateMemberVisible(ctx context.Context, owner, traceID, name string, visible bool) (*MemberView, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > MaxMemberNameLen {
		return nil, bizerr.New(bizerr.CodeInvalidArgument, "成员昵称需为 1~32 个字符")
	}
	if containsDirectIdentifier(name) != "" {
		return nil, bizerr.New(bizerr.CodeInvalidArgument, "请使用昵称或角色（如「我」「配偶」），不要填写真实姓名或证件信息")
	}
	var view *MemberView
	err := s.repo.Transaction(ctx, func(tx *repository.Repository) error {
		if err := s.ensureConsent(ctx, tx, owner); err != nil {
			return err
		}
		count, err := tx.CountMembers(ctx, owner)
		if err != nil {
			return err
		}
		if count >= MaxMembers {
			return bizerr.New(bizerr.CodeInvalidArgument, "成员数量已达上限")
		}
		dek, keyVersion, err := s.dekFor(ctx, tx, owner)
		if err != nil {
			return err
		}
		rowUID, err := security.NewRowUID()
		if err != nil {
			return err
		}
		now := s.nowUTC()
		row := &domain.LedgerMember{
			OwnerRef: owner, RowUID: rowUID, SortOrder: int(count),
			Hidden: !visible, KeyVersion: keyVersion, CreatedAt: now, UpdatedAt: now,
		}
		if row.NameCipher, err = s.keys.Seal(dek, []byte(name), security.BuildAAD(owner, rowUID, "name")); err != nil {
			return err
		}
		row.RowMAC = s.keys.RowMAC(s.memberRow(row).Canonical())
		if err := tx.CreateMember(ctx, row); err != nil {
			return err
		}
		if err := tx.AppendAudit(ctx, owner, "member.create", "member", strconv.FormatUint(row.ID, 10), traceID); err != nil {
			return err
		}
		view = &MemberView{ID: row.ID, Name: name, SortOrder: row.SortOrder, Visible: visible}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return view, nil
}

// RenameMember 重命名成员（保持原显示状态）。
func (s *Service) RenameMember(ctx context.Context, owner, traceID string, id uint64, name string) (*MemberView, error) {
	return s.UpdateMember(ctx, owner, traceID, id, MemberInput{Name: &name})
}

// UpdateMember 修改成员昵称与「是否在记账中显示」。
//
// 只改显示状态时不重新加密昵称，但仍刷新 UpdatedAt 与行 MAC，
// 保持行级完整性字段与库内时间一致。
func (s *Service) UpdateMember(ctx context.Context, owner, traceID string, id uint64, in MemberInput) (*MemberView, error) {
	if in.Name == nil && in.Visible == nil {
		return nil, bizerr.New(bizerr.CodeInvalidArgument, "请提供要修改的成员字段")
	}
	var name string
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
		if name == "" || len([]rune(name)) > MaxMemberNameLen {
			return nil, bizerr.New(bizerr.CodeInvalidArgument, "成员昵称需为 1~32 个字符")
		}
		if containsDirectIdentifier(name) != "" {
			return nil, bizerr.New(bizerr.CodeInvalidArgument, "请使用昵称或角色，不要填写真实姓名或证件信息")
		}
	}
	var view *MemberView
	err := s.repo.Transaction(ctx, func(tx *repository.Repository) error {
		if err := s.ensureConsent(ctx, tx, owner); err != nil {
			return err
		}
		row, err := tx.GetMember(ctx, owner, id)
		if err != nil {
			return err
		}
		if row == nil {
			return notFound()
		}
		row.UpdatedAt = s.nowUTC()
		if in.Name != nil {
			dek, keyVersion, err := s.dekFor(ctx, tx, owner)
			if err != nil {
				return err
			}
			row.KeyVersion = keyVersion
			if row.NameCipher, err = s.keys.Seal(dek, []byte(name), security.BuildAAD(owner, row.RowUID, "name")); err != nil {
				return err
			}
		}
		if in.Visible != nil {
			row.Hidden = !*in.Visible
		}
		row.RowMAC = s.keys.RowMAC(s.memberRow(row).Canonical())
		if err := tx.UpdateMember(ctx, row); err != nil {
			return err
		}
		if err := tx.AppendAudit(ctx, owner, "member.update", "member", strconv.FormatUint(id, 10), traceID); err != nil {
			return err
		}
		if in.Name == nil {
			// 未改名时返回库内昵称需要解密。
			if name, err = s.openMemberByID(ctx, tx, owner, row); err != nil {
				return err
			}
		}
		view = &MemberView{
			ID: row.ID, Name: name, SortOrder: row.SortOrder, Visible: !row.Hidden,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return view, nil
}

// openMemberByID 在事务内按当前密钥派生 DEK 并解密成员昵称。
func (s *Service) openMemberByID(ctx context.Context, tx *repository.Repository, owner string, row *domain.LedgerMember) (string, error) {
	dek, _, err := s.dekFor(ctx, tx, owner)
	if err != nil {
		return "", err
	}
	return s.openMember(dek, row)
}

// DeleteMember 删除成员标签。
//
// 条目上的 member_id 不改写：行级 MAC 覆盖该字段，静默改写会使历史条目
// 校验失败；条目内的姓名密文快照即历史归属的唯一真源，删除成员不影响
// 既有条目的展示与完整性。
func (s *Service) DeleteMember(ctx context.Context, owner, traceID string, id uint64) error {
	return s.repo.Transaction(ctx, func(tx *repository.Repository) error {
		if err := s.ensureConsent(ctx, tx, owner); err != nil {
			return err
		}
		row, err := tx.GetMember(ctx, owner, id)
		if err != nil {
			return err
		}
		if row == nil {
			return notFound()
		}
		if err := tx.DeleteMember(ctx, owner, id); err != nil {
			return err
		}
		return tx.AppendAudit(ctx, owner, "member.delete", "member", strconv.FormatUint(id, 10), traceID)
	})
}

// openMember 校验行 MAC 后解密成员昵称。
func (s *Service) openMember(dek []byte, row *domain.LedgerMember) (string, error) {
	if !s.keys.VerifyMemberMAC(s.memberRow(row), row.RowMAC) {
		return "", bizerr.New(bizerr.CodeInternal, "数据完整性校验失败")
	}
	plain, err := s.keys.Open(dek, row.NameCipher, security.BuildAAD(row.OwnerRef, row.RowUID, "name"))
	if err != nil {
		return "", bizerr.New(bizerr.CodeInternal, "数据解密失败")
	}
	return string(plain), nil
}

func (s *Service) memberRow(row *domain.LedgerMember) security.MemberRow {
	return security.MemberRow{
		OwnerRef: row.OwnerRef, RowUID: row.RowUID, SortOrder: row.SortOrder,
		KeyVersion: row.KeyVersion, UpdatedAt: row.UpdatedAt, NameCipher: row.NameCipher,
	}
}
