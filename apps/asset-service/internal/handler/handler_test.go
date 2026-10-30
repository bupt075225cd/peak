package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"peak/apps/asset-service/internal/repository"
	"peak/apps/asset-service/internal/service"
	"peak/libs/domain"
	"peak/libs/security"
)

// env 测试环境：SQLite 空库 + 真实仓储/服务/处理器。
type env struct {
	router *gin.Engine
	db     *gorm.DB
}

func setup(t *testing.T) *env {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := domain.OpenDB(domain.DialectSQLite, filepath.Join(t.TempDir(), "ledger.db"), logger.Silent)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := domain.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	keys, err := security.LoadKeys(security.KeyConfig{AllowDev: true})
	if err != nil {
		t.Fatalf("load keys: %v", err)
	}
	svc := service.New(repository.New(db), keys, service.Options{
		ReauthWindow: 5 * time.Minute, MaxItems: 20, DeletionGraceDays: 30,
	})
	h := New(svc)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("trace_id", "test-trace")
		c.Next()
	})
	h.RegisterRoutes(r)
	return &env{router: r, db: db}
}

type apiResp struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// call 以指定用户身份发起请求；iat 为空表示不带新鲜令牌头。
func (e *env) call(t *testing.T, method, path string, uid uint64, iat string, body any) (*httptest.ResponseRecorder, apiResp) {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", strconv.FormatUint(uid, 10))
	if iat != "" {
		req.Header.Set("X-Token-Iat", iat)
	}
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)

	var resp apiResp
	if w.Body.Len() > 0 && strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
	}
	return w, resp
}

func nowIat() string { return strconv.FormatInt(time.Now().Unix(), 10) }

func (e *env) consent(t *testing.T, uid uint64) {
	t.Helper()
	w, _ := e.call(t, http.MethodPost, "/api/ledger/privacy/consent", uid, "", map[string]string{})
	if w.Code != http.StatusOK {
		t.Fatalf("consent failed: %d %s", w.Code, w.Body.String())
	}
}

func (e *env) createItem(t *testing.T, uid uint64, name string, cents int64) uint64 {
	t.Helper()
	w, resp := e.call(t, http.MethodPost, "/api/ledger/items", uid, "", map[string]any{
		"name": name, "category": "liquid", "sub_type": "bank_current",
		"amount_cents": cents, "currency": "CNY", "risk_type": "cash",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("create item: %d %s", w.Code, w.Body.String())
	}
	var item struct {
		ID      uint64 `json:"id"`
		Version uint32 `json:"version"`
	}
	if err := json.Unmarshal(resp.Data, &item); err != nil {
		t.Fatalf("decode item: %v", err)
	}
	if item.Version != 1 {
		t.Fatalf("version = %d, want 1", item.Version)
	}
	return item.ID
}

func TestConsentRequiredBeforeData(t *testing.T) {
	e := setup(t)

	w, resp := e.call(t, http.MethodGet, "/api/ledger/summary", 1, "", nil)
	if w.Code != http.StatusForbidden || resp.Code != 1003 {
		t.Fatalf("summary without consent = %d/%d, want 403/1003", w.Code, resp.Code)
	}
	w, _ = e.call(t, http.MethodPost, "/api/ledger/items", 1, "", map[string]any{"name": "现金", "category": "liquid", "amount_cents": 100})
	if w.Code != http.StatusForbidden {
		t.Fatalf("create without consent = %d, want 403", w.Code)
	}

	e.consent(t, 1)
	w, _ = e.call(t, http.MethodGet, "/api/ledger/summary", 1, "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("summary after consent = %d %s", w.Code, w.Body.String())
	}
}

func TestCreateItemAndSummary(t *testing.T) {
	e := setup(t)
	e.consent(t, 1)
	e.createItem(t, 1, "招商银行活期", 2_200_000)
	e.createItem(t, 1, "自住房产", 5_000_000_00)

	w, resp := e.call(t, http.MethodGet, "/api/ledger/summary", 1, "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("summary: %d %s", w.Code, w.Body.String())
	}
	var summary struct {
		TotalAssetCents     int64 `json:"total_asset_cents"`
		TotalLiabilityCents int64 `json:"total_liability_cents"`
		ItemCount           int   `json:"item_count"`
		Groups              []struct {
			Category string `json:"category"`
			Children []struct {
				Key string `json:"key"`
			} `json:"children"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(resp.Data, &summary); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	want := int64(2_200_000 + 5_000_000_00)
	if summary.TotalAssetCents != want || summary.ItemCount != 2 {
		t.Fatalf("summary = %+v, want total %d and 2 items", summary, want)
	}
	if summary.TotalLiabilityCents != 0 {
		t.Fatalf("liability = %d, want 0", summary.TotalLiabilityCents)
	}
}

func TestSummaryFilteredByMemberQuery(t *testing.T) {
	e := setup(t)
	e.consent(t, 9)

	w, resp := e.call(t, http.MethodPost, "/api/ledger/members", 9, "", map[string]string{"name": "配偶"})
	if w.Code != http.StatusOK {
		t.Fatalf("create member: %d %s", w.Code, w.Body.String())
	}
	var member struct {
		ID uint64 `json:"id"`
	}
	if err := json.Unmarshal(resp.Data, &member); err != nil {
		t.Fatalf("decode member: %v", err)
	}

	e.createItem(t, 9, "共同存款", 600_000)
	w, _ = e.call(t, http.MethodPost, "/api/ledger/items", 9, "", map[string]any{
		"name": "配偶基金", "category": "investment", "sub_type": "fund",
		"amount_cents": 400_000, "member_id": member.ID,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("create member item: %d %s", w.Code, w.Body.String())
	}

	type summaryResp struct {
		TotalAssetCents int64 `json:"total_asset_cents"`
		ItemCount       int   `json:"item_count"`
	}
	decode := func(resp apiResp) summaryResp {
		var s summaryResp
		if err := json.Unmarshal(resp.Data, &s); err != nil {
			t.Fatalf("decode summary: %v", err)
		}
		return s
	}

	_, resp = e.call(t, http.MethodGet, "/api/ledger/summary", 9, "", nil)
	if all := decode(resp); all.ItemCount != 2 || all.TotalAssetCents != 1_000_000 {
		t.Fatalf("unfiltered summary = %+v", all)
	}

	_, resp = e.call(t, http.MethodGet, "/api/ledger/summary?member_id="+strconv.FormatUint(member.ID, 10), 9, "", nil)
	if one := decode(resp); one.ItemCount != 1 || one.TotalAssetCents != 400_000 {
		t.Fatalf("member summary = %+v", one)
	}

	// 「我」= 未归属任何成员的条目。
	_, resp = e.call(t, http.MethodGet, "/api/ledger/summary?member_id=self", 9, "", nil)
	if self := decode(resp); self.ItemCount != 1 || self.TotalAssetCents != 600_000 {
		t.Fatalf("self summary = %+v", self)
	}

	w, _ = e.call(t, http.MethodGet, "/api/ledger/summary?member_id=abc", 9, "", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad member_id status = %d, want 400", w.Code)
	}
}

// 成员的「在记账中显示」开关：只影响展示，成员与其名下条目都保留。
func TestMemberVisibilityToggle(t *testing.T) {
	e := setup(t)
	e.consent(t, 15)

	w, resp := e.call(t, http.MethodPost, "/api/ledger/members", 15, "", map[string]any{
		"name": "配偶", "visible": false,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("create member: %d %s", w.Code, w.Body.String())
	}
	var member struct {
		ID      uint64 `json:"id"`
		Name    string `json:"name"`
		Visible bool   `json:"visible"`
	}
	if err := json.Unmarshal(resp.Data, &member); err != nil {
		t.Fatalf("decode member: %v", err)
	}
	if member.Visible {
		t.Fatalf("new member should be hidden: %+v", member)
	}

	w, resp = e.call(t, http.MethodPut, "/api/ledger/members/"+strconv.FormatUint(member.ID, 10), 15, "",
		map[string]any{"visible": true})
	if w.Code != http.StatusOK {
		t.Fatalf("toggle visible: %d %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(resp.Data, &member); err != nil {
		t.Fatalf("decode member: %v", err)
	}
	// 仅切换显示状态时昵称保持不变（需要解密回填）。
	if !member.Visible || member.Name != "配偶" {
		t.Fatalf("toggle result = %+v", member)
	}

	_, resp = e.call(t, http.MethodGet, "/api/ledger/members", 15, "", nil)
	var list struct {
		Items []struct {
			Visible bool `json:"visible"`
		} `json:"items"`
	}
	if err := json.Unmarshal(resp.Data, &list); err != nil {
		t.Fatalf("decode members: %v", err)
	}
	if len(list.Items) != 1 || !list.Items[0].Visible {
		t.Fatalf("member list = %+v", list.Items)
	}
}

func TestSensitiveFieldsEncryptedAtRest(t *testing.T) {
	e := setup(t)
	e.consent(t, 2)
	const secretName = "张三家传收藏品"
	const note = "保险柜第二层"
	id := e.createItem(t, 2, secretName, 123_456_00)

	var row domain.LedgerItem
	if err := e.db.Where("id = ?", id).First(&row).Error; err != nil {
		t.Fatalf("load row: %v", err)
	}
	for _, field := range [][]byte{row.NameCipher, row.AmtCipher, row.NoteCipher, row.MemCipher} {
		if len(field) == 0 {
			t.Fatal("ciphertext column is empty")
		}
		if bytes.Contains(field, []byte(secretName)) || bytes.Contains(field, []byte(note)) {
			t.Fatal("ciphertext must not contain plaintext")
		}
	}
	if bytes.Contains(row.NameCipher, []byte("12345600")) {
		t.Fatal("amount must not appear in ciphertext")
	}
	// 伪名不含原始用户 ID，且长度固定。
	if len(row.OwnerRef) != 64 || strings.Contains(row.OwnerRef, "|2|") {
		t.Fatalf("owner_ref looks wrong: %q", row.OwnerRef)
	}
	if len(row.RowMAC) == 0 {
		t.Fatal("row mac missing")
	}
	// 明文注释不应出现在任何列中。
	var raw struct {
		NoteCipher []byte
	}
	if err := e.db.Model(&domain.LedgerItem{}).Select("note_cipher").Where("id = ?", id).Scan(&raw).Error; err != nil {
		t.Fatalf("scan: %v", err)
	}
	if bytes.Contains(raw.NoteCipher, []byte(note)) {
		t.Fatal("note plaintext leaked")
	}
}

func TestUpdateVersionConflict(t *testing.T) {
	e := setup(t)
	e.consent(t, 3)
	id := e.createItem(t, 3, "股票账户", 6_290_000)

	body := map[string]any{
		"name": "股票账户", "category": "investment", "sub_type": "stock",
		"amount_cents": 9_000_000, "currency": "CNY", "risk_type": "long", "version": 1,
	}
	w, _ := e.call(t, http.MethodPut, "/api/ledger/items/"+strconv.FormatUint(id, 10), 3, "", body)
	if w.Code != http.StatusOK {
		t.Fatalf("update: %d %s", w.Code, w.Body.String())
	}
	// 版本已变为 2，旧版本应冲突。
	w, resp := e.call(t, http.MethodPut, "/api/ledger/items/"+strconv.FormatUint(id, 10), 3, "", body)
	if w.Code != http.StatusConflict || resp.Code != 1005 {
		t.Fatalf("stale update = %d/%d, want 409/1005", w.Code, resp.Code)
	}
}

func TestCrossUserAccessIsNotFound(t *testing.T) {
	e := setup(t)
	e.consent(t, 4)
	e.consent(t, 5)
	id := e.createItem(t, 4, "私有资产", 1_000_00)
	path := "/api/ledger/items/" + strconv.FormatUint(id, 10)

	w, resp := e.call(t, http.MethodPut, path, 5, "", map[string]any{
		"name": "改", "category": "liquid", "amount_cents": 1, "version": 1,
	})
	if w.Code != http.StatusNotFound || resp.Code != 1004 {
		t.Fatalf("cross-user update = %d/%d, want 404/1004", w.Code, resp.Code)
	}
	w, _ = e.call(t, http.MethodDelete, path, 5, "", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-user delete = %d, want 404", w.Code)
	}
}

func TestExportRequiresFreshToken(t *testing.T) {
	e := setup(t)
	e.consent(t, 6)
	e.createItem(t, 6, "基金账户", 900_000)

	w, resp := e.call(t, http.MethodGet, "/api/ledger/privacy/export", 6, "", nil)
	if w.Code != http.StatusPreconditionRequired || resp.Code != 1007 {
		t.Fatalf("export without iat = %d/%d, want 428/1007", w.Code, resp.Code)
	}
	// 过期令牌同样拒绝。
	stale := strconv.FormatInt(time.Now().Add(-time.Hour).Unix(), 10)
	w, _ = e.call(t, http.MethodGet, "/api/ledger/privacy/export", 6, stale, nil)
	if w.Code != http.StatusPreconditionRequired {
		t.Fatalf("export with stale iat = %d, want 428", w.Code)
	}

	w, _ = e.call(t, http.MethodGet, "/api/ledger/privacy/export", 6, nowIat(), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("export = %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "900000") {
		t.Fatal("export should contain amount as cents")
	}
	if disposition := w.Header().Get("Content-Disposition"); !strings.Contains(disposition, "attachment") {
		t.Fatalf("unexpected disposition: %q", disposition)
	}
}

func TestMembersLifecycle(t *testing.T) {
	e := setup(t)
	e.consent(t, 7)

	w, resp := e.call(t, http.MethodPost, "/api/ledger/members", 7, "", map[string]string{"name": "配偶"})
	if w.Code != http.StatusOK {
		t.Fatalf("create member: %d %s", w.Code, w.Body.String())
	}
	var member struct {
		ID uint64 `json:"id"`
	}
	if err := json.Unmarshal(resp.Data, &member); err != nil {
		t.Fatalf("decode member: %v", err)
	}

	w, _ = e.call(t, http.MethodPost, "/api/ledger/items", 7, "", map[string]any{
		"name": "共同基金", "category": "investment", "sub_type": "fund",
		"amount_cents": 500_000, "member_id": member.ID,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("create item with member: %d %s", w.Code, w.Body.String())
	}

	w, _ = e.call(t, http.MethodDelete, "/api/ledger/members/"+strconv.FormatUint(member.ID, 10), 7, "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("delete member: %d %s", w.Code, w.Body.String())
	}

	_, resp = e.call(t, http.MethodGet, "/api/ledger/items", 7, "", nil)
	var list struct {
		Items []struct {
			MemberID   *uint64 `json:"member_id"`
			MemberName string  `json:"member_name"`
		} `json:"items"`
	}
	if err := json.Unmarshal(resp.Data, &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(list.Items))
	}
	// 成员删除后，条目保留历史归属快照（行 MAC 覆盖 member_id，不做静默改写）。
	if list.Items[0].MemberName != "配偶" {
		t.Fatalf("member name snapshot = %q, want 配偶", list.Items[0].MemberName)
	}
}

func TestSnapshotsUpsertedOnWrite(t *testing.T) {
	e := setup(t)
	e.consent(t, 8)
	id := e.createItem(t, 8, "活期", 1_000_00)

	w, resp := e.call(t, http.MethodGet, "/api/ledger/snapshots", 8, "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("snapshots: %d %s", w.Code, w.Body.String())
	}
	var snaps struct {
		Items []struct {
			Period  string `json:"period"`
			Payload struct {
				TotalAssetCents int64 `json:"total_asset_cents"`
			} `json:"payload"`
		} `json:"items"`
	}
	if err := json.Unmarshal(resp.Data, &snaps); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(snaps.Items) != 1 || snaps.Items[0].Payload.TotalAssetCents != 1_000_00 {
		t.Fatalf("snapshots = %+v", snaps.Items)
	}

	if w, _ = e.call(t, http.MethodDelete, "/api/ledger/items/"+strconv.FormatUint(id, 10), 8, "", nil); w.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", w.Code, w.Body.String())
	}
	_, resp = e.call(t, http.MethodGet, "/api/ledger/snapshots", 8, "", nil)
	_ = json.Unmarshal(resp.Data, &snaps)
	if len(snaps.Items) != 1 || snaps.Items[0].Payload.TotalAssetCents != 0 {
		t.Fatalf("snapshot after delete = %+v", snaps.Items)
	}
}

func TestPurgeDestroysKeyring(t *testing.T) {
	e := setup(t)
	e.consent(t, 9)
	e.createItem(t, 9, "待删除资产", 1_000_00)

	w, _ := e.call(t, http.MethodPost, "/api/ledger/privacy/deletion/purge", 9, "", nil)
	if w.Code != http.StatusPreconditionRequired {
		t.Fatalf("purge without fresh token = %d, want 428", w.Code)
	}
	w, _ = e.call(t, http.MethodPost, "/api/ledger/privacy/deletion/purge", 9, nowIat(), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("purge = %d %s", w.Code, w.Body.String())
	}

	var keyrings, items int64
	e.db.Model(&domain.LedgerKeyring{}).Count(&keyrings)
	e.db.Model(&domain.LedgerItem{}).Count(&items)
	if keyrings != 0 || items != 0 {
		t.Fatalf("after purge keyrings=%d items=%d, want 0/0", keyrings, items)
	}
	// 同意记录同时清除：再次访问需重新同意。
	w, resp := e.call(t, http.MethodGet, "/api/ledger/summary", 9, "", nil)
	if w.Code != http.StatusForbidden || resp.Code != 1003 {
		t.Fatalf("post-purge summary = %d/%d, want 403/1003", w.Code, resp.Code)
	}
}

func TestDeletionLifecycle(t *testing.T) {
	e := setup(t)
	e.consent(t, 10)

	w, resp := e.call(t, http.MethodPost, "/api/ledger/privacy/deletion", 10, nowIat(), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("request deletion: %d %s", w.Code, w.Body.String())
	}
	var view struct {
		Status   string `json:"status"`
		DaysLeft int    `json:"days_left"`
	}
	if err := json.Unmarshal(resp.Data, &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if view.Status != domain.LedgerDeletionPending || view.DaysLeft != 30 {
		t.Fatalf("deletion view = %+v", view)
	}

	w, resp = e.call(t, http.MethodDelete, "/api/ledger/privacy/deletion", 10, "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("cancel deletion: %d %s", w.Code, w.Body.String())
	}
	_ = json.Unmarshal(resp.Data, &view)
	if view.Status != domain.LedgerDeletionCancelled {
		t.Fatalf("status = %q, want cancelled", view.Status)
	}
}

func TestRejectsDirectIdentifiers(t *testing.T) {
	e := setup(t)
	e.consent(t, 11)

	w, resp := e.call(t, http.MethodPost, "/api/ledger/items", 11, "", map[string]any{
		"name": "招商银行卡 6222021234567890123", "category": "liquid", "amount_cents": 100,
	})
	if w.Code != http.StatusBadRequest || resp.Code != 1001 {
		t.Fatalf("direct identifier = %d/%d, want 400/1001", w.Code, resp.Code)
	}
}

func TestReadOnlyEndpoints(t *testing.T) {
	e := setup(t)
	e.consent(t, 12)

	// 元数据。
	w, resp := e.call(t, http.MethodGet, "/api/ledger/meta", 12, "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("meta: %d %s", w.Code, w.Body.String())
	}
	var meta struct {
		ConsentRequired bool `json:"consent_required"`
		Categories      []struct {
			Key string `json:"key"`
		} `json:"categories"`
	}
	if err := json.Unmarshal(resp.Data, &meta); err != nil {
		t.Fatalf("decode meta: %v", err)
	}
	if meta.ConsentRequired || len(meta.Categories) != 5 {
		t.Fatalf("meta = %+v", meta)
	}

	// 成员列表（空）。
	w, _ = e.call(t, http.MethodGet, "/api/ledger/members", 12, "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("members: %d", w.Code)
	}
	// 同意状态。
	w, _ = e.call(t, http.MethodGet, "/api/ledger/privacy/consent", 12, "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("consent status: %d", w.Code)
	}
	// 注销状态（未申请）。
	w, _ = e.call(t, http.MethodGet, "/api/ledger/privacy/deletion", 12, "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("deletion status: %d", w.Code)
	}
	// 分页参数非法时回退默认值。
	w, _ = e.call(t, http.MethodGet, "/api/ledger/items?offset=-5&limit=abc", 12, "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list with bad paging: %d", w.Code)
	}
	// 年份非法。
	w, resp = e.call(t, http.MethodGet, "/api/ledger/snapshots?year=26", 12, "", nil)
	if w.Code != http.StatusBadRequest || resp.Code != 1001 {
		t.Fatalf("bad year = %d/%d", w.Code, resp.Code)
	}
}

func TestMemberRenameAndValidation(t *testing.T) {
	e := setup(t)
	e.consent(t, 13)

	w, resp := e.call(t, http.MethodPost, "/api/ledger/members", 13, "", map[string]string{"name": "我"})
	if w.Code != http.StatusOK {
		t.Fatalf("create member: %d %s", w.Code, w.Body.String())
	}
	var member struct {
		ID   uint64 `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(resp.Data, &member); err != nil {
		t.Fatalf("decode member: %v", err)
	}

	path := "/api/ledger/members/" + strconv.FormatUint(member.ID, 10)
	w, resp = e.call(t, http.MethodPut, path, 13, "", map[string]string{"name": "本人"})
	if w.Code != http.StatusOK {
		t.Fatalf("rename: %d %s", w.Code, w.Body.String())
	}
	_ = json.Unmarshal(resp.Data, &member)
	if member.Name != "本人" {
		t.Fatalf("renamed = %+v", member)
	}

	// 非法 ID / 非法 JSON 走参数错误分支。
	w, resp = e.call(t, http.MethodPut, "/api/ledger/members/abc", 13, "", map[string]string{"name": "x"})
	if w.Code != http.StatusBadRequest || resp.Code != 1001 {
		t.Fatalf("bad id = %d/%d", w.Code, resp.Code)
	}
	w, _ = e.call(t, http.MethodPost, "/api/ledger/members", 13, "", "not-an-object")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad json = %d", w.Code)
	}
	w, _ = e.call(t, http.MethodPost, "/api/ledger/items", 13, "", "not-an-object")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad item json = %d", w.Code)
	}
	w, _ = e.call(t, http.MethodPut, path, 13, "", map[string]string{"name": strings.Repeat("长", 40)})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("long member name = %d", w.Code)
	}
}

func TestMissingIdentityRejected(t *testing.T) {
	e := setup(t)
	req := httptest.NewRequest(http.MethodGet, "/api/ledger/meta", nil)
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("missing identity = %d, want 401", w.Code)
	}
}
