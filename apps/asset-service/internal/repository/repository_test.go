package repository

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/gorm/logger"

	"peak/libs/domain"
)

func newRepo(t *testing.T) *Repository {
	t.Helper()
	db, err := domain.OpenDB(domain.DialectSQLite, filepath.Join(t.TempDir(), "repo.db"), logger.Silent)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := domain.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return New(db)
}

func sampleItem(owner string) *domain.LedgerItem {
	now := time.Now().UTC().Truncate(time.Second)
	return &domain.LedgerItem{
		OwnerRef: owner, RowUID: "row-" + owner, Category: domain.LedgerCategoryLiquid,
		Currency: "CNY", RiskType: domain.LedgerRiskCash, KeyVersion: 1, Version: 1,
		NameCipher: []byte("n"), SubCipher: []byte("s"), AmtCipher: []byte("a"),
		NoteCipher: []byte("t"), MemCipher: []byte("m"), RowMAC: []byte("mac"),
		CreatedAt: now, UpdatedAt: now,
	}
}

func TestKeyringLifecycle(t *testing.T) {
	r := newRepo(t)
	ctx := context.Background()

	got, err := r.GetKeyring(ctx, "owner-a")
	if err != nil || got != nil {
		t.Fatalf("missing keyring = %v, %v", got, err)
	}
	key := &domain.LedgerKeyring{OwnerRef: "owner-a", Salt: []byte("0123456789abcdef0123456789abcdef"), KeyVersion: 1}
	if err := r.CreateKeyring(ctx, key); err != nil {
		t.Fatalf("create keyring: %v", err)
	}
	got, err = r.GetKeyring(ctx, "owner-a")
	if err != nil || got == nil || string(got.Salt) != string(key.Salt) {
		t.Fatalf("get keyring = %+v, %v", got, err)
	}
	if err := r.DeleteKeyring(ctx, "owner-a"); err != nil {
		t.Fatalf("delete keyring: %v", err)
	}
	if got, _ = r.GetKeyring(ctx, "owner-a"); got != nil {
		t.Fatal("keyring should be deleted")
	}
}

func TestItemCRUD(t *testing.T) {
	r := newRepo(t)
	ctx := context.Background()
	item := sampleItem("owner-b")

	if err := r.CreateItem(ctx, item); err != nil {
		t.Fatalf("create: %v", err)
	}
	if item.ID == 0 {
		t.Fatal("id not assigned")
	}
	got, err := r.GetItem(ctx, "owner-b", item.ID)
	if err != nil || got == nil || got.RowUID != item.RowUID {
		t.Fatalf("get = %+v, %v", got, err)
	}
	// 跨主体查询视为不存在。
	if other, _ := r.GetItem(ctx, "owner-x", item.ID); other != nil {
		t.Fatal("cross-owner get must be nil")
	}

	if n, err := r.CountItems(ctx, "owner-b"); err != nil || n != 1 {
		t.Fatalf("count = %d, %v", n, err)
	}
	list, err := r.ListItems(ctx, "owner-b")
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %d, %v", len(list), err)
	}

	item.Version = 2
	item.AmtCipher = []byte("a2")
	item.UpdatedAt = item.UpdatedAt.Add(time.Minute)
	ok, err := r.UpdateItem(ctx, item, 1)
	if err != nil || !ok {
		t.Fatalf("update = %v, %v", ok, err)
	}
	// 旧版本再次更新应失配。
	if ok, _ = r.UpdateItem(ctx, item, 1); ok {
		t.Fatal("stale update should not match")
	}
	got, _ = r.GetItem(ctx, "owner-b", item.ID)
	if got.Version != 2 || string(got.AmtCipher) != "a2" {
		t.Fatalf("updated row = %+v", got)
	}

	if err := r.DeleteItem(ctx, got); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if n, _ := r.CountItems(ctx, "owner-b"); n != 0 {
		t.Fatalf("count after delete = %d", n)
	}
}

func TestMemberCRUDAndClearRef(t *testing.T) {
	r := newRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	member := &domain.LedgerMember{
		OwnerRef: "owner-c", RowUID: "mrow", NameCipher: []byte("n"), SortOrder: 0,
		KeyVersion: 1, RowMAC: []byte("m"), CreatedAt: now, UpdatedAt: now,
	}
	if err := r.CreateMember(ctx, member); err != nil {
		t.Fatalf("create member: %v", err)
	}
	got, err := r.GetMember(ctx, "owner-c", member.ID)
	if err != nil || got == nil {
		t.Fatalf("get member = %+v, %v", got, err)
	}
	if other, _ := r.GetMember(ctx, "owner-z", member.ID); other != nil {
		t.Fatal("cross-owner member must be nil")
	}
	if n, _ := r.CountMembers(ctx, "owner-c"); n != 1 {
		t.Fatalf("count members = %d", n)
	}
	members, err := r.ListMembers(ctx, "owner-c")
	if err != nil || len(members) != 1 {
		t.Fatalf("list members = %d, %v", len(members), err)
	}

	member.NameCipher = []byte("n2")
	member.UpdatedAt = now.Add(time.Minute)
	if err := r.UpdateMember(ctx, member); err != nil {
		t.Fatalf("update member: %v", err)
	}

	item := sampleItem("owner-c")
	item.MemberID = &member.ID
	if err := r.CreateItem(ctx, item); err != nil {
		t.Fatalf("create item: %v", err)
	}
	if err := r.ClearMemberRef(ctx, "owner-c", member.ID); err != nil {
		t.Fatalf("clear ref: %v", err)
	}
	gotItem, _ := r.GetItem(ctx, "owner-c", item.ID)
	if gotItem.MemberID != nil {
		t.Fatalf("member_id = %v, want nil", gotItem.MemberID)
	}

	if err := r.DeleteMember(ctx, "owner-c", member.ID); err != nil {
		t.Fatalf("delete member: %v", err)
	}
	if n, _ := r.CountMembers(ctx, "owner-c"); n != 0 {
		t.Fatalf("count after delete = %d", n)
	}
}

func TestSnapshots(t *testing.T) {
	r := newRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	snap := &domain.LedgerSnapshot{OwnerRef: "owner-d", Period: "2026-01", PayloadCipher: []byte("p1"), KeyVersion: 1, UpdatedAt: now}
	if err := r.UpsertSnapshot(ctx, snap); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := r.GetSnapshot(ctx, "owner-d", "2026-01")
	if err != nil || got == nil || string(got.PayloadCipher) != "p1" {
		t.Fatalf("get snapshot = %+v, %v", got, err)
	}
	// 同一周期覆盖而不是新增。
	snap.PayloadCipher = []byte("p2")
	if err := r.UpsertSnapshot(ctx, snap); err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	list, err := r.ListSnapshots(ctx, "owner-d", "2026-")
	if err != nil || len(list) != 1 || string(list[0].PayloadCipher) != "p2" {
		t.Fatalf("list snapshots = %+v, %v", list, err)
	}
	if missing, _ := r.GetSnapshot(ctx, "owner-d", "2026-02"); missing != nil {
		t.Fatal("missing snapshot must be nil")
	}
}

func TestAuditChain(t *testing.T) {
	r := newRepo(t)
	ctx := context.Background()

	for _, action := range []string{"item.create", "item.update", "item.delete"} {
		if err := r.AppendAudit(ctx, "owner-e", action, "item", "1", "trace"); err != nil {
			t.Fatalf("append %s: %v", action, err)
		}
	}
	logs, err := r.ListAuditLogs(ctx, "owner-e")
	if err != nil || len(logs) != 3 {
		t.Fatalf("logs = %d, %v", len(logs), err)
	}
	if logs[0].PrevHash != "" {
		t.Fatal("first entry prev hash should be empty")
	}
	for i := 1; i < len(logs); i++ {
		if logs[i].PrevHash != logs[i-1].Hash {
			t.Fatalf("chain broken at %d", i)
		}
		if logs[i].Hash == "" {
			t.Fatalf("hash missing at %d", i)
		}
	}
	// 不同主体链相互独立。
	if err := r.AppendAudit(ctx, "owner-f", "item.create", "item", "9", ""); err != nil {
		t.Fatalf("append other owner: %v", err)
	}
	others, _ := r.ListAuditLogs(ctx, "owner-f")
	if len(others) != 1 || others[0].PrevHash != "" {
		t.Fatalf("other owner chain = %+v", others)
	}
}

func TestConsentAndDeletion(t *testing.T) {
	r := newRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	if got, err := r.GetConsent(ctx, "owner-g", "v1"); err != nil || got != nil {
		t.Fatalf("missing consent = %v, %v", got, err)
	}
	consent := &domain.LedgerConsent{OwnerRef: "owner-g", PolicyVersion: "v1", IPHash: "h", ConsentedAt: now}
	if err := r.CreateConsent(ctx, consent); err != nil {
		t.Fatalf("create consent: %v", err)
	}
	// 幂等：重复写入不报错。
	if err := r.CreateConsent(ctx, consent); err != nil {
		t.Fatalf("duplicate consent: %v", err)
	}
	if got, _ := r.GetConsent(ctx, "owner-g", "v1"); got == nil {
		t.Fatal("consent should exist")
	}

	req := &domain.LedgerDeletionRequest{
		OwnerRef: "owner-g", Status: domain.LedgerDeletionPending,
		RequestedAt: now, ExecuteAfter: now.Add(-time.Hour), UpdatedAt: now,
	}
	if err := r.UpsertDeletion(ctx, req); err != nil {
		t.Fatalf("upsert deletion: %v", err)
	}
	due, err := r.ListDueDeletions(ctx, time.Now(), 10)
	if err != nil || len(due) != 1 {
		t.Fatalf("due = %d, %v", len(due), err)
	}

	req.Status = domain.LedgerDeletionCancelled
	if err := r.UpsertDeletion(ctx, req); err != nil {
		t.Fatalf("cancel deletion: %v", err)
	}
	if got, _ := r.GetDeletion(ctx, "owner-g"); got == nil || got.Status != domain.LedgerDeletionCancelled {
		t.Fatalf("deletion = %+v", got)
	}
	if due, _ = r.ListDueDeletions(ctx, time.Now(), 10); len(due) != 0 {
		t.Fatalf("cancelled request should not be due: %d", len(due))
	}
}

func TestPurgeOwner(t *testing.T) {
	r := newRepo(t)
	ctx := context.Background()
	owner := "owner-h"

	if err := r.CreateKeyring(ctx, &domain.LedgerKeyring{OwnerRef: owner, Salt: []byte("salt"), KeyVersion: 1}); err != nil {
		t.Fatalf("keyring: %v", err)
	}
	if err := r.CreateItem(ctx, sampleItem(owner)); err != nil {
		t.Fatalf("item: %v", err)
	}
	now := time.Now().UTC()
	if err := r.CreateMember(ctx, &domain.LedgerMember{OwnerRef: owner, RowUID: "r", SortOrder: 0, KeyVersion: 1, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("member: %v", err)
	}
	if err := r.UpsertSnapshot(ctx, &domain.LedgerSnapshot{OwnerRef: owner, Period: "2026-01", PayloadCipher: []byte("p"), KeyVersion: 1, UpdatedAt: now}); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if err := r.AppendAudit(ctx, owner, "item.create", "item", "1", ""); err != nil {
		t.Fatalf("audit: %v", err)
	}
	if err := r.CreateConsent(ctx, &domain.LedgerConsent{OwnerRef: owner, PolicyVersion: "v1", ConsentedAt: now}); err != nil {
		t.Fatalf("consent: %v", err)
	}
	if err := r.UpsertDeletion(ctx, &domain.LedgerDeletionRequest{OwnerRef: owner, Status: domain.LedgerDeletionPending, RequestedAt: now, ExecuteAfter: now}); err != nil {
		t.Fatalf("deletion: %v", err)
	}

	if err := r.PurgeOwner(ctx, owner); err != nil {
		t.Fatalf("purge: %v", err)
	}
	if keyring, _ := r.GetKeyring(ctx, owner); keyring != nil {
		t.Fatal("keyring must be purged")
	}
	if n, _ := r.CountItems(ctx, owner); n != 0 {
		t.Fatalf("items left: %d", n)
	}
	if logs, _ := r.ListAuditLogs(ctx, owner); len(logs) != 0 {
		t.Fatalf("audit left: %d", len(logs))
	}
}

func TestTransactionRollback(t *testing.T) {
	r := newRepo(t)
	ctx := context.Background()

	err := r.Transaction(ctx, func(tx *Repository) error {
		if err := tx.CreateItem(ctx, sampleItem("owner-i")); err != nil {
			return err
		}
		return context.Canceled
	})
	if err == nil {
		t.Fatal("transaction should return error")
	}
	if n, _ := r.CountItems(ctx, "owner-i"); n != 0 {
		t.Fatalf("rollback failed, items = %d", n)
	}
}

func TestDeleteAllItems(t *testing.T) {
	r := newRepo(t)
	ctx := context.Background()
	if err := r.CreateItem(ctx, sampleItem("owner-j")); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := r.DeleteAllItems(ctx, "owner-j"); err != nil {
		t.Fatalf("delete all: %v", err)
	}
	if n, _ := r.CountItems(ctx, "owner-j"); n != 0 {
		t.Fatalf("items left = %d", n)
	}
}
