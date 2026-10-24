package service

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm/logger"

	"peak/apps/asset-service/internal/repository"
	"peak/libs/domain"
	bizerr "peak/libs/errors"
	"peak/libs/security"
)

func newService(t *testing.T) (*Service, *repository.Repository) {
	t.Helper()
	db, err := domain.OpenDB(domain.DialectSQLite, filepath.Join(t.TempDir(), "svc.db"), logger.Silent)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := domain.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	keys, err := security.LoadKeys(security.KeyConfig{AllowDev: true})
	if err != nil {
		t.Fatalf("keys: %v", err)
	}
	repo := repository.New(db)
	return New(repo, keys, Options{ReauthWindow: time.Minute, MaxItems: 3, DeletionGraceDays: 30}), repo
}

func consented(t *testing.T, svc *Service) (context.Context, string) {
	t.Helper()
	ctx := context.Background()
	owner := svc.OwnerRef(1)
	if _, err := svc.AcceptConsent(ctx, owner, domain.LedgerConsentPolicyVersion, "127.0.0.1", "trace"); err != nil {
		t.Fatalf("consent: %v", err)
	}
	return ctx, owner
}

func validInput() ItemInput {
	return ItemInput{
		Name: "招商银行活期", Category: domain.LedgerCategoryLiquid, SubType: "bank_current",
		AmountCents: 2_200_000, Currency: domain.LedgerCurrencyCNY, RiskType: domain.LedgerRiskCash,
	}
}

func TestOwnerRefStable(t *testing.T) {
	svc, _ := newService(t)
	first := svc.OwnerRef(7)
	second := svc.OwnerRef(7)
	other := svc.OwnerRef(8)
	if first != second || first == other {
		t.Fatal("owner ref must be stable and unique")
	}
	if svc.ReauthWindow() != time.Minute {
		t.Fatalf("reauth window = %v", svc.ReauthWindow())
	}
}

func TestMetaAndConsentFlow(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	owner := svc.OwnerRef(2)

	meta, err := svc.Meta(ctx, owner)
	if err != nil {
		t.Fatalf("meta: %v", err)
	}
	if !meta.ConsentRequired || meta.ConsentPolicyVersion == "" || len(meta.Categories) != 5 {
		t.Fatalf("meta = %+v", meta)
	}
	if _, err := svc.Summary(ctx, owner); bizerr.CodeOf(err) != bizerr.CodeForbidden {
		t.Fatalf("summary before consent err = %v", err)
	}

	status, err := svc.ConsentStatus(ctx, owner)
	if err != nil || !status.Required {
		t.Fatalf("consent status = %+v, %v", status, err)
	}
	if _, err := svc.AcceptConsent(ctx, owner, "outdated-version", "ip", "t"); bizerr.CodeOf(err) != bizerr.CodeInvalidArgument {
		t.Fatalf("stale policy version err = %v", err)
	}
	accepted, err := svc.AcceptConsent(ctx, owner, domain.LedgerConsentPolicyVersion, "ip", "t")
	if err != nil || accepted.Required || accepted.ConsentedAt == nil {
		t.Fatalf("accepted = %+v, %v", accepted, err)
	}
	meta, _ = svc.Meta(ctx, owner)
	if meta.ConsentRequired {
		t.Fatal("consent should no longer be required")
	}
}

func TestValidation(t *testing.T) {
	svc, _ := newService(t)
	ctx, owner := consented(t, svc)

	cases := []struct {
		name string
		mut  func(*ItemInput)
	}{
		{"empty name", func(in *ItemInput) { in.Name = "  " }},
		{"zero amount", func(in *ItemInput) { in.AmountCents = 0 }},
		{"too large", func(in *ItemInput) { in.AmountCents = MaxAmountCents + 1 }},
		{"bad category", func(in *ItemInput) { in.Category = "unknown" }},
		{"bad subtype", func(in *ItemInput) { in.SubType = "mortgage" }},
		{"bad currency", func(in *ItemInput) { in.Currency = "BTC" }},
		{"bad risk", func(in *ItemInput) { in.RiskType = "wild" }},
		{"long note", func(in *ItemInput) { in.Note = strings.Repeat("好", MaxNoteLen+1) }},
		{"id card", func(in *ItemInput) { in.Name = "身份证 110101199003072316" }},
	}
	for _, tc := range cases {
		in := validInput()
		tc.mut(&in)
		if _, err := svc.CreateItem(ctx, owner, "t", in); bizerr.CodeOf(err) != bizerr.CodeInvalidArgument {
			t.Fatalf("%s: err = %v, want invalid argument", tc.name, err)
		}
	}
}

func TestItemLifecycleAndAggregation(t *testing.T) {
	svc, _ := newService(t)
	ctx, owner := consented(t, svc)

	in := validInput()
	created, err := svc.CreateItem(ctx, owner, "t", in)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Name != in.Name || created.AmountCents != in.AmountCents || created.Version != 1 {
		t.Fatalf("created = %+v", created)
	}

	liability := validInput()
	liability.Name = "房屋贷款"
	liability.Category = domain.LedgerCategoryLiability
	liability.SubType = "mortgage"
	liability.AmountCents = 1_528_000_00
	if _, err := svc.CreateItem(ctx, owner, "t", liability); err != nil {
		t.Fatalf("create liability: %v", err)
	}

	summary, err := svc.Summary(ctx, owner)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary.TotalAssetCents != in.AmountCents || summary.TotalLiabilityCents != liability.AmountCents {
		t.Fatalf("summary totals = %+v", summary)
	}
	if summary.NetAssetCents != summary.TotalAssetCents-summary.TotalLiabilityCents {
		t.Fatal("net asset mismatch")
	}
	if summary.ItemCount != 2 || summary.UpdatedAt.IsZero() {
		t.Fatalf("summary = %+v", summary)
	}

	list, err := svc.ListItems(ctx, owner, ItemFilter{Category: domain.LedgerCategoryLiability})
	if err != nil || list.Total != 1 || list.SumCents != liability.AmountCents {
		t.Fatalf("filtered list = %+v, %v", list, err)
	}
	if list.CategoryCounts[domain.LedgerCategoryLiquid] != 1 {
		t.Fatalf("category counts = %+v", list.CategoryCounts)
	}
	// 关键词在解密后匹配。
	list, _ = svc.ListItems(ctx, owner, ItemFilter{Keyword: "招商"})
	if list.Total != 1 {
		t.Fatalf("keyword total = %d", list.Total)
	}
	// 越界分页返回空切片而非 panic。
	list, _ = svc.ListItems(ctx, owner, ItemFilter{Offset: 99, Limit: 200})
	if list.Total != 2 || len(list.Items) != 0 {
		t.Fatalf("paged list = %+v", list)
	}

	// 版本冲突。
	update := UpdateInput{ItemInput: in, Version: 1}
	update.AmountCents = 3_000_000
	if _, err := svc.UpdateItem(ctx, owner, "t", created.ID, update); err != nil {
		t.Fatalf("update: %v", err)
	}
	if _, err := svc.UpdateItem(ctx, owner, "t", created.ID, update); bizerr.CodeOf(err) != bizerr.CodeConflict {
		t.Fatalf("stale update err = %v", err)
	}
	// 越权/不存在。
	if _, err := svc.UpdateItem(ctx, owner, "t", 9999, update); bizerr.CodeOf(err) != bizerr.CodeNotFound {
		t.Fatalf("missing update err = %v", err)
	}
	if err := svc.DeleteItem(ctx, owner, "t", 9999); bizerr.CodeOf(err) != bizerr.CodeNotFound {
		t.Fatalf("missing delete err = %v", err)
	}
	if err := svc.DeleteItem(ctx, owner, "t", created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func TestItemLimit(t *testing.T) {
	svc, _ := newService(t)
	ctx, owner := consented(t, svc)
	for i := 0; i < 3; i++ {
		if _, err := svc.CreateItem(ctx, owner, "t", validInput()); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}
	if _, err := svc.CreateItem(ctx, owner, "t", validInput()); bizerr.CodeOf(err) != bizerr.CodeInvalidArgument {
		t.Fatalf("limit err = %v", err)
	}
}

func TestSnapshotsAndYearValidation(t *testing.T) {
	svc, _ := newService(t)
	ctx, owner := consented(t, svc)
	if _, err := svc.CreateItem(ctx, owner, "t", validInput()); err != nil {
		t.Fatalf("create: %v", err)
	}
	year := time.Now().UTC().Format("2006")
	snaps, err := svc.Snapshots(ctx, owner, year)
	if err != nil || len(snaps) != 1 {
		t.Fatalf("snapshots = %d, %v", len(snaps), err)
	}
	if snaps[0].Payload.TotalAssetCents != validInput().AmountCents {
		t.Fatalf("snapshot payload = %+v", snaps[0].Payload)
	}
	if _, err := svc.Snapshots(ctx, owner, "26"); bizerr.CodeOf(err) != bizerr.CodeInvalidArgument {
		t.Fatalf("bad year err = %v", err)
	}
	// 默认年份。
	if _, err := svc.Snapshots(ctx, owner, ""); err != nil {
		t.Fatalf("default year: %v", err)
	}
}

func TestMemberLifecycle(t *testing.T) {
	svc, _ := newService(t)
	ctx, owner := consented(t, svc)

	if _, err := svc.CreateMember(ctx, owner, "t", "  "); bizerr.CodeOf(err) != bizerr.CodeInvalidArgument {
		t.Fatalf("empty member err = %v", err)
	}
	if _, err := svc.CreateMember(ctx, owner, "t", "110101199003072316"); bizerr.CodeOf(err) != bizerr.CodeInvalidArgument {
		t.Fatalf("id card member err = %v", err)
	}
	member, err := svc.CreateMember(ctx, owner, "t", "配偶")
	if err != nil {
		t.Fatalf("create member: %v", err)
	}
	list, err := svc.ListMembers(ctx, owner)
	if err != nil || len(list) != 1 || list[0].Name != "配偶" {
		t.Fatalf("members = %+v, %v", list, err)
	}

	in := validInput()
	in.MemberID = &member.ID
	item, err := svc.CreateItem(ctx, owner, "t", in)
	if err != nil || item.MemberName != "配偶" {
		t.Fatalf("item with member = %+v, %v", item, err)
	}
	// 不存在的成员。
	missing := uint64(9999)
	bad := validInput()
	bad.MemberID = &missing
	if _, err := svc.CreateItem(ctx, owner, "t", bad); bizerr.CodeOf(err) != bizerr.CodeInvalidArgument {
		t.Fatalf("missing member err = %v", err)
	}

	renamed, err := svc.RenameMember(ctx, owner, "t", member.ID, "爱人")
	if err != nil || renamed.Name != "爱人" {
		t.Fatalf("rename = %+v, %v", renamed, err)
	}
	if _, err := svc.RenameMember(ctx, owner, "t", 9999, "x"); bizerr.CodeOf(err) != bizerr.CodeNotFound {
		t.Fatalf("rename missing err = %v", err)
	}
	if err := svc.DeleteMember(ctx, owner, "t", member.ID); err != nil {
		t.Fatalf("delete member: %v", err)
	}
	if err := svc.DeleteMember(ctx, owner, "t", member.ID); bizerr.CodeOf(err) != bizerr.CodeNotFound {
		t.Fatalf("delete missing member err = %v", err)
	}
}

func TestMemberLimit(t *testing.T) {
	svc, _ := newService(t)
	ctx, owner := consented(t, svc)
	for i := 0; i < MaxMembers; i++ {
		if _, err := svc.CreateMember(ctx, owner, "t", "成员"); err != nil {
			t.Fatalf("create member %d: %v", i, err)
		}
	}
	if _, err := svc.CreateMember(ctx, owner, "t", "超出"); bizerr.CodeOf(err) != bizerr.CodeInvalidArgument {
		t.Fatalf("member limit err = %v", err)
	}
}

func TestExportAndDeletion(t *testing.T) {
	svc, repo := newService(t)
	ctx, owner := consented(t, svc)
	if _, err := svc.CreateItem(ctx, owner, "t", validInput()); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.CreateMember(ctx, owner, "t", "我"); err != nil {
		t.Fatalf("member: %v", err)
	}

	raw, filename, err := svc.Export(ctx, owner, "t")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if !strings.Contains(string(raw), "招商银行活期") || !strings.Contains(filename, "家庭资产账本") {
		t.Fatalf("export payload/filename unexpected: %s / %s", filename, raw[:min(len(raw), 80)])
	}
	// 导出写入审计。
	logs, _ := repo.ListAuditLogs(ctx, owner)
	found := false
	for _, l := range logs {
		if l.Action == "privacy.export" {
			found = true
		}
	}
	if !found {
		t.Fatal("export audit missing")
	}

	status, err := svc.DeletionStatus(ctx, owner)
	if err != nil || status.Status != "none" {
		t.Fatalf("deletion status = %+v, %v", status, err)
	}
	requested, err := svc.RequestDeletion(ctx, owner, "t")
	if err != nil || requested.Status != domain.LedgerDeletionPending || requested.DaysLeft != 30 {
		t.Fatalf("request deletion = %+v, %v", requested, err)
	}
	status, _ = svc.DeletionStatus(ctx, owner)
	if status.Status != domain.LedgerDeletionPending {
		t.Fatalf("status = %+v", status)
	}
	cancelled, err := svc.CancelDeletion(ctx, owner, "t")
	if err != nil || cancelled.Status != domain.LedgerDeletionCancelled {
		t.Fatalf("cancel = %+v, %v", cancelled, err)
	}
	// 已撤销后再次撤销是幂等的。
	if again, err := svc.CancelDeletion(ctx, owner, "t"); err != nil || again.Status != "none" {
		t.Fatalf("cancel again = %+v, %v", again, err)
	}
}

func TestPurgeNowAndDue(t *testing.T) {
	svc, repo := newService(t)
	ctx, owner := consented(t, svc)
	if _, err := svc.CreateItem(ctx, owner, "t", validInput()); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.PurgeNow(ctx, owner, "t"); err != nil {
		t.Fatalf("purge now: %v", err)
	}
	if keyring, err := repo.GetKeyring(ctx, owner); err != nil || keyring != nil {
		t.Fatalf("keyring should be removed: %+v, %v", keyring, err)
	}

	// 到期清扫：直接写入一条已到期申请。
	other := svc.OwnerRef(99)
	if _, err := svc.AcceptConsent(ctx, other, domain.LedgerConsentPolicyVersion, "ip", "t"); err != nil {
		t.Fatalf("consent other: %v", err)
	}
	now := time.Now().UTC()
	if err := repo.UpsertDeletion(ctx, &domain.LedgerDeletionRequest{
		OwnerRef: other, Status: domain.LedgerDeletionPending,
		RequestedAt: now.AddDate(0, 0, -40), ExecuteAfter: now.Add(-time.Hour), UpdatedAt: now,
	}); err != nil {
		t.Fatalf("upsert deletion: %v", err)
	}
	purged, err := svc.PurgeDue(ctx)
	if err != nil || purged != 1 {
		t.Fatalf("purge due = %d, %v", purged, err)
	}
}

func TestLoadItemsEmpty(t *testing.T) {
	svc, _ := newService(t)
	ctx, owner := consented(t, svc)
	items, err := svc.loadItems(ctx, svc.repo, owner)
	if err != nil || len(items) != 0 {
		t.Fatalf("empty load = %d, %v", len(items), err)
	}
	summary, err := svc.Summary(ctx, owner)
	if err != nil || summary.ItemCount != 0 || len(summary.Groups) != 5 {
		t.Fatalf("empty summary = %+v, %v", summary, err)
	}
}

func TestMacTamperDetected(t *testing.T) {
	svc, repo := newService(t)
	ctx, owner := consented(t, svc)
	created, err := svc.CreateItem(ctx, owner, "t", validInput())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// 直接改写密文列，模拟数据库层静默篡改。
	if err := repo.Transaction(ctx, func(tx *repository.Repository) error {
		row, err := tx.GetItem(ctx, owner, created.ID)
		if err != nil {
			return err
		}
		row.AmtCipher = append([]byte(nil), row.AmtCipher...)
		row.AmtCipher[len(row.AmtCipher)-1] ^= 0xff
		_, err = tx.UpdateItem(ctx, row, row.Version)
		return err
	}); err != nil {
		t.Fatalf("tamper: %v", err)
	}
	if _, err := svc.ListItems(ctx, owner, ItemFilter{}); bizerr.CodeOf(err) != bizerr.CodeInternal {
		t.Fatalf("tampered read err = %v, want internal", err)
	}
}

func TestNewDefaultsAndNormalization(t *testing.T) {
	_, repo := newService(t)
	keys, err := security.LoadKeys(security.KeyConfig{AllowDev: true})
	if err != nil {
		t.Fatalf("keys: %v", err)
	}
	svc := New(repo, keys, Options{})
	if svc.opts.MaxItems != 2000 || svc.opts.DeletionGraceDays != domain.LedgerDeletionGraceDays {
		t.Fatalf("defaults = %+v", svc.opts)
	}
	if svc.ReauthWindow() != 5*time.Minute {
		t.Fatalf("reauth window = %v", svc.ReauthWindow())
	}

	ctx, owner := consented(t, svc)
	// 省略子类/币种/资产类型时应归一化为默认值。
	item, err := svc.CreateItem(ctx, owner, "t", ItemInput{Name: "现金", Category: domain.LedgerCategoryLiquid, AmountCents: 100})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if item.SubType != "bank_current" || item.Currency != domain.LedgerCurrencyCNY || item.RiskType != domain.LedgerRiskNone {
		t.Fatalf("normalized = %+v", item)
	}
}

func TestDirectIdentifierPatterns(t *testing.T) {
	cases := map[string]string{
		"110101199003072316":  "疑似身份证号",
		"6222021234567890123": "疑似银行卡号",
		"13800001111":         "疑似手机号",
	}
	for input, want := range cases {
		if got := containsDirectIdentifier("备注 " + input); got != want {
			t.Fatalf("containsDirectIdentifier(%q) = %q, want %q", input, got, want)
		}
	}
	if containsDirectIdentifier("招商银行活期 22 万") != "" {
		t.Fatal("normal text should not be flagged")
	}
}

func TestMemberMacTamperDetected(t *testing.T) {
	svc, repo := newService(t)
	ctx, owner := consented(t, svc)
	member, err := svc.CreateMember(ctx, owner, "t", "配偶")
	if err != nil {
		t.Fatalf("create member: %v", err)
	}
	if err := repo.Transaction(ctx, func(tx *repository.Repository) error {
		row, err := tx.GetMember(ctx, owner, member.ID)
		if err != nil {
			return err
		}
		row.NameCipher = append([]byte(nil), row.NameCipher...)
		row.NameCipher[len(row.NameCipher)-1] ^= 0xff
		return tx.UpdateMember(ctx, row)
	}); err != nil {
		t.Fatalf("tamper: %v", err)
	}
	if _, err := svc.ListMembers(ctx, owner); bizerr.CodeOf(err) != bizerr.CodeInternal {
		t.Fatalf("tampered member read err = %v", err)
	}
}

func TestSortItemsStableOnEqualTime(t *testing.T) {
	fixed := time.Unix(1_700_000_000, 0).UTC()
	items := []ItemView{
		{ID: 1, UpdatedAt: fixed},
		{ID: 3, UpdatedAt: fixed},
		{ID: 2, UpdatedAt: fixed.Add(time.Minute)},
	}
	sortItems(items)
	if items[0].ID != 2 || items[1].ID != 3 || items[2].ID != 1 {
		t.Fatalf("sort order = %d,%d,%d", items[0].ID, items[1].ID, items[2].ID)
	}
}

func TestExportWithoutConsent(t *testing.T) {
	svc, _ := newService(t)
	if _, _, err := svc.Export(context.Background(), svc.OwnerRef(77), "t"); bizerr.CodeOf(err) != bizerr.CodeForbidden {
		t.Fatalf("export without consent err = %v", err)
	}
}

func TestUpdateWithoutVersionSkipsConflict(t *testing.T) {
	svc, _ := newService(t)
	ctx, owner := consented(t, svc)
	created, err := svc.CreateItem(ctx, owner, "t", validInput())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	in := validInput()
	in.AmountCents = 9_900_000
	updated, err := svc.UpdateItem(ctx, owner, "t", created.ID, UpdateInput{ItemInput: in})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.AmountCents != in.AmountCents || updated.Version != created.Version+1 {
		t.Fatalf("updated = %+v", updated)
	}
}
