// Package repository 账本数据访问（GORM）。
//
// 所有方法都以 owner_ref（HMAC 伪名）为隔离维度，绝不接受裸 user_id；
// 密文列只做存取，不解密、不参与 SQL 聚合与模糊检索。
package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"peak/libs/domain"
)

// Repository 账本仓储。
type Repository struct {
	db *gorm.DB

	// 审计链追加按 owner 串行化：链头读取与写入之间存在竞争，
	// 进程内互斥 + 乐观条件更新可保证单实例下链不分叉
	// （多实例部署需改为数据库行锁或分布式锁）。
	auditMu    sync.Mutex
	auditLocks map[string]*sync.Mutex
}

// New 创建仓储。
func New(db *gorm.DB) *Repository {
	return &Repository{db: db, auditLocks: map[string]*sync.Mutex{}}
}

// Transaction 在事务中执行，回调收到绑定同一事务的仓储副本。
func (r *Repository) Transaction(ctx context.Context, fn func(tx *Repository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&Repository{db: tx, auditLocks: r.auditLocks})
	})
}

// ── 密钥盐 ────────────────────────────────────────────────

// GetKeyring 读取每用户密钥盐；不存在返回 nil, nil。
func (r *Repository) GetKeyring(ctx context.Context, owner string) (*domain.LedgerKeyring, error) {
	var key domain.LedgerKeyring
	err := r.db.WithContext(ctx).Where("owner_ref = ?", owner).First(&key).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &key, nil
}

// CreateKeyring 写入每用户密钥盐。
func (r *Repository) CreateKeyring(ctx context.Context, key *domain.LedgerKeyring) error {
	return r.db.WithContext(ctx).Create(key).Error
}

// DeleteKeyring 删除密钥盐（密码学删除的关键一步）。
func (r *Repository) DeleteKeyring(ctx context.Context, owner string) error {
	return r.db.WithContext(ctx).Where("owner_ref = ?", owner).Delete(&domain.LedgerKeyring{}).Error
}

// ── 条目 ──────────────────────────────────────────────────

// ListItems 读取该用户全部条目（解密与聚合在 service 层完成）。
func (r *Repository) ListItems(ctx context.Context, owner string) ([]domain.LedgerItem, error) {
	var items []domain.LedgerItem
	err := r.db.WithContext(ctx).Where("owner_ref = ?", owner).Find(&items).Error
	return items, err
}

// GetItem 按归属查询单条；不存在或不属于该用户均返回 nil, nil（不泄露存在性）。
func (r *Repository) GetItem(ctx context.Context, owner string, id uint64) (*domain.LedgerItem, error) {
	var item domain.LedgerItem
	err := r.db.WithContext(ctx).Where("owner_ref = ? AND id = ?", owner, id).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// CountItems 统计条目数（用于软上限判断）。
func (r *Repository) CountItems(ctx context.Context, owner string) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&domain.LedgerItem{}).Where("owner_ref = ?", owner).Count(&n).Error
	return n, err
}

// CreateItem 新增条目。
func (r *Repository) CreateItem(ctx context.Context, item *domain.LedgerItem) error {
	return r.db.WithContext(ctx).Create(item).Error
}

// UpdateItem 以 version 为条件更新；返回是否命中（未命中视为并发冲突）。
func (r *Repository) UpdateItem(ctx context.Context, item *domain.LedgerItem, expectVersion uint32) (bool, error) {
	res := r.db.WithContext(ctx).Model(&domain.LedgerItem{}).
		Where("id = ? AND owner_ref = ? AND version = ?", item.ID, item.OwnerRef, expectVersion).
		Updates(map[string]any{
			"category":    item.Category,
			"name_cipher": item.NameCipher,
			"sub_cipher":  item.SubCipher,
			"amt_cipher":  item.AmtCipher,
			"note_cipher": item.NoteCipher,
			"mem_cipher":  item.MemCipher,
			"member_id":   item.MemberID,
			"currency":    item.Currency,
			"risk_type":   item.RiskType,
			"key_version": item.KeyVersion,
			"version":     item.Version,
			"row_mac":     item.RowMAC,
		})
	if res.Error != nil {
		return false, res.Error
	}
	// updated_at 需显式写入并与行 MAC 覆盖的取值一致。
	if res.RowsAffected == 1 {
		if err := r.db.WithContext(ctx).Model(&domain.LedgerItem{}).
			Where("id = ? AND owner_ref = ?", item.ID, item.OwnerRef).
			UpdateColumn("updated_at", item.UpdatedAt).Error; err != nil {
			return false, err
		}
	}
	return res.RowsAffected == 1, nil
}

// DeleteItem 软删除条目。
func (r *Repository) DeleteItem(ctx context.Context, item *domain.LedgerItem) error {
	return r.db.WithContext(ctx).Where("id = ? AND owner_ref = ?", item.ID, item.OwnerRef).
		Delete(&domain.LedgerItem{}).Error
}

// ClearMemberRef 把条目对某成员的引用置空（成员删除时调用，保留密文姓名快照）。
func (r *Repository) ClearMemberRef(ctx context.Context, owner string, memberID uint64) error {
	return r.db.WithContext(ctx).Model(&domain.LedgerItem{}).
		Where("owner_ref = ? AND member_id = ?", owner, memberID).
		Update("member_id", nil).Error
}

// ── 成员 ──────────────────────────────────────────────────

// ListMembers 列出成员。
func (r *Repository) ListMembers(ctx context.Context, owner string) ([]domain.LedgerMember, error) {
	var members []domain.LedgerMember
	err := r.db.WithContext(ctx).Where("owner_ref = ?", owner).
		Order("sort_order ASC, id ASC").Find(&members).Error
	return members, err
}

// GetMember 按归属查询成员。
func (r *Repository) GetMember(ctx context.Context, owner string, id uint64) (*domain.LedgerMember, error) {
	var member domain.LedgerMember
	err := r.db.WithContext(ctx).Where("owner_ref = ? AND id = ?", owner, id).First(&member).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &member, nil
}

// CountMembers 统计成员数（上限 20）。
func (r *Repository) CountMembers(ctx context.Context, owner string) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&domain.LedgerMember{}).Where("owner_ref = ?", owner).Count(&n).Error
	return n, err
}

// CreateMember 新增成员。
func (r *Repository) CreateMember(ctx context.Context, member *domain.LedgerMember) error {
	return r.db.WithContext(ctx).Create(member).Error
}

// UpdateMember 更新成员（名称密文 + 显示状态 + 行 MAC）。
func (r *Repository) UpdateMember(ctx context.Context, member *domain.LedgerMember) error {
	return r.db.WithContext(ctx).Model(&domain.LedgerMember{}).
		Where("id = ? AND owner_ref = ?", member.ID, member.OwnerRef).
		Updates(map[string]any{
			"name_cipher": member.NameCipher,
			"sort_order":  member.SortOrder,
			"hidden":      member.Hidden,
			"key_version": member.KeyVersion,
			"row_mac":     member.RowMAC,
			"updated_at":  member.UpdatedAt,
		}).Error
}

// DeleteMember 软删除成员。
func (r *Repository) DeleteMember(ctx context.Context, owner string, id uint64) error {
	return r.db.WithContext(ctx).Where("id = ? AND owner_ref = ?", id, owner).
		Delete(&domain.LedgerMember{}).Error
}

// ── 月度快照 ──────────────────────────────────────────────

// GetSnapshot 读取指定月份快照。
func (r *Repository) GetSnapshot(ctx context.Context, owner, period string) (*domain.LedgerSnapshot, error) {
	var snap domain.LedgerSnapshot
	err := r.db.WithContext(ctx).Where("owner_ref = ? AND period = ?", owner, period).First(&snap).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &snap, nil
}

// ListSnapshots 列出指定前缀（如 "2026-"）的快照。
func (r *Repository) ListSnapshots(ctx context.Context, owner, periodPrefix string) ([]domain.LedgerSnapshot, error) {
	var snaps []domain.LedgerSnapshot
	err := r.db.WithContext(ctx).Where("owner_ref = ? AND period LIKE ?", owner, periodPrefix+"%").
		Order("period ASC").Find(&snaps).Error
	return snaps, err
}

// UpsertSnapshot 按 (owner_ref, period) 覆盖写入。
func (r *Repository) UpsertSnapshot(ctx context.Context, snap *domain.LedgerSnapshot) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "owner_ref"}, {Name: "period"}},
		DoUpdates: clause.Assignments(map[string]any{
			"payload_cipher": snap.PayloadCipher,
			"key_version":    snap.KeyVersion,
			"updated_at":     snap.UpdatedAt,
		}),
	}).Create(snap).Error
}

// ── 审计链 ────────────────────────────────────────────────

// AppendAudit 追加一条审计事件并推进链头。
//
// 只记录动作与目标 ID，不含金额、名称等敏感内容；哈希链使"删除中间一条"
// 或"改写历史动作"可被检测。
func (r *Repository) AppendAudit(ctx context.Context, owner, action, targetType, targetID, traceID string) error {
	lock := r.ownerLock(owner)
	lock.Lock()
	defer lock.Unlock()

	for attempt := 0; attempt < 3; attempt++ {
		head, err := r.getAuditHead(ctx, owner)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		prev := ""
		var lastID uint64
		if head != nil {
			prev = head.LastHash
			lastID = head.LastID
		}
		entry := domain.LedgerAuditLog{
			OwnerRef: owner, Action: action, TargetType: targetType,
			TargetID: targetID, TraceID: traceID, PrevHash: prev, CreatedAt: now,
		}
		entry.Hash = auditHash(prev, owner, action, targetType, targetID, traceID, now)
		if err := r.db.WithContext(ctx).Create(&entry).Error; err != nil {
			return err
		}

		if head == nil {
			newHead := domain.LedgerAuditHead{OwnerRef: owner, LastHash: entry.Hash, LastID: entry.ID, UpdatedAt: now}
			err = r.db.WithContext(ctx).Create(&newHead).Error
			if err == nil {
				return nil
			}
		} else {
			res := r.db.WithContext(ctx).Model(&domain.LedgerAuditHead{}).
				Where("owner_ref = ? AND last_id = ?", owner, lastID).
				Updates(map[string]any{"last_hash": entry.Hash, "last_id": entry.ID, "updated_at": now})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 1 {
				return nil
			}
		}
		// 链头竞争失败：回滚本条后重试。
		_ = r.db.WithContext(ctx).Delete(&domain.LedgerAuditLog{}, entry.ID).Error
	}
	return nil
}

// ListAuditLogs 读取审计日志（按时间正序，供导出与自检）。
func (r *Repository) ListAuditLogs(ctx context.Context, owner string) ([]domain.LedgerAuditLog, error) {
	var logs []domain.LedgerAuditLog
	err := r.db.WithContext(ctx).Where("owner_ref = ?", owner).Order("id ASC").Find(&logs).Error
	return logs, err
}

func (r *Repository) getAuditHead(ctx context.Context, owner string) (*domain.LedgerAuditHead, error) {
	var head domain.LedgerAuditHead
	err := r.db.WithContext(ctx).Where("owner_ref = ?", owner).First(&head).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &head, nil
}

func (r *Repository) ownerLock(owner string) *sync.Mutex {
	r.auditMu.Lock()
	defer r.auditMu.Unlock()
	lock, ok := r.auditLocks[owner]
	if !ok {
		lock = &sync.Mutex{}
		r.auditLocks[owner] = lock
	}
	return lock
}

func auditHash(prev, owner, action, targetType, targetID, traceID string, at time.Time) string {
	h := sha256.New()
	for _, part := range []string{prev, owner, action, targetType, targetID, traceID, strconv.FormatInt(at.UnixNano(), 10)} {
		h.Write([]byte(part))
		h.Write([]byte{0x1f})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// ── 同意留痕 ──────────────────────────────────────────────

// GetConsent 读取指定政策版本的同意记录。
func (r *Repository) GetConsent(ctx context.Context, owner, version string) (*domain.LedgerConsent, error) {
	var consent domain.LedgerConsent
	err := r.db.WithContext(ctx).Where("owner_ref = ? AND policy_version = ?", owner, version).First(&consent).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &consent, nil
}

// CreateConsent 写入同意记录（幂等：已存在则忽略冲突）。
func (r *Repository) CreateConsent(ctx context.Context, consent *domain.LedgerConsent) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(consent).Error
}

// ── 注销申请 ──────────────────────────────────────────────

// GetDeletion 读取注销申请。
func (r *Repository) GetDeletion(ctx context.Context, owner string) (*domain.LedgerDeletionRequest, error) {
	var req domain.LedgerDeletionRequest
	err := r.db.WithContext(ctx).Where("owner_ref = ?", owner).First(&req).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &req, nil
}

// UpsertDeletion 覆盖写入注销申请。
func (r *Repository) UpsertDeletion(ctx context.Context, req *domain.LedgerDeletionRequest) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "owner_ref"}},
		DoUpdates: clause.Assignments(map[string]any{
			"status":        req.Status,
			"requested_at":  req.RequestedAt,
			"execute_after": req.ExecuteAfter,
			"updated_at":    req.UpdatedAt,
		}),
	}).Create(req).Error
}

// ListDueDeletions 列出已到期待清除的申请。
//
// 时间比较放到进程内完成：SQLite 与 PostgreSQL 的时间字面量格式不同，
// 直接比较可能因字符串序错判（跨方言一致性优先，且待清理记录量很小）。
func (r *Repository) ListDueDeletions(ctx context.Context, now time.Time, limit int) ([]domain.LedgerDeletionRequest, error) {
	var pending []domain.LedgerDeletionRequest
	if err := r.db.WithContext(ctx).
		Where("status = ?", domain.LedgerDeletionPending).
		Limit(limit * 4).Find(&pending).Error; err != nil {
		return nil, err
	}
	due := make([]domain.LedgerDeletionRequest, 0, len(pending))
	for _, req := range pending {
		if req.ExecuteAfter.After(now) {
			continue
		}
		due = append(due, req)
		if len(due) >= limit {
			break
		}
	}
	return due, nil
}

// ── 密码学删除 ────────────────────────────────────────────

// PurgeOwner 物理删除该用户全部账本数据，并移除密钥盐。
// 密钥盐删除后，即使数据库备份仍含密文也无法解密。
func (r *Repository) PurgeOwner(ctx context.Context, owner string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		steps := []func() error{
			func() error {
				return tx.Unscoped().Where("owner_ref = ?", owner).Delete(&domain.LedgerItem{}).Error
			},
			func() error {
				return tx.Unscoped().Where("owner_ref = ?", owner).Delete(&domain.LedgerMember{}).Error
			},
			func() error {
				return tx.Unscoped().Where("owner_ref = ?", owner).Delete(&domain.LedgerSnapshot{}).Error
			},
			func() error {
				return tx.Unscoped().Where("owner_ref = ?", owner).Delete(&domain.LedgerAuditLog{}).Error
			},
			func() error {
				return tx.Unscoped().Where("owner_ref = ?", owner).Delete(&domain.LedgerAuditHead{}).Error
			},
			func() error {
				return tx.Unscoped().Where("owner_ref = ?", owner).Delete(&domain.LedgerConsent{}).Error
			},
			// 最后删除密钥盐：此步之后遗留密文永久不可解。
			func() error {
				return tx.Unscoped().Where("owner_ref = ?", owner).Delete(&domain.LedgerKeyring{}).Error
			},
			func() error {
				return tx.Unscoped().Where("owner_ref = ?", owner).Delete(&domain.LedgerDeletionRequest{}).Error
			},
		}
		for _, step := range steps {
			if err := step(); err != nil {
				return err
			}
		}
		return nil
	})
}

// DeleteAllItems 清空该用户全部条目（保留密钥盐与成员）。
func (r *Repository) DeleteAllItems(ctx context.Context, owner string) error {
	return r.db.WithContext(ctx).Unscoped().Where("owner_ref = ?", owner).Delete(&domain.LedgerItem{}).Error
}
