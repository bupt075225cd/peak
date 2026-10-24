package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"peak/libs/domain"
	bizerr "peak/libs/errors"
)

// ConsentView 单独同意状态。
type ConsentView struct {
	PolicyVersion string     `json:"policy_version"`
	Required      bool       `json:"required"`
	ConsentedAt   *time.Time `json:"consented_at"`
}

// ConsentStatus 查询当前政策版本的同意状态。
func (s *Service) ConsentStatus(ctx context.Context, owner string) (*ConsentView, error) {
	consent, err := s.repo.GetConsent(ctx, owner, domain.LedgerConsentPolicyVersion)
	if err != nil {
		return nil, err
	}
	view := &ConsentView{PolicyVersion: domain.LedgerConsentPolicyVersion, Required: consent == nil}
	if consent != nil {
		at := consent.ConsentedAt
		view.ConsentedAt = &at
	}
	return view, nil
}

// AcceptConsent 记录敏感个人信息处理的单独同意（仅存 IP 哈希）。
func (s *Service) AcceptConsent(ctx context.Context, owner, version, ip, traceID string) (*ConsentView, error) {
	if version != "" && version != domain.LedgerConsentPolicyVersion {
		return nil, bizerr.New(bizerr.CodeInvalidArgument, "告知版本已更新，请刷新页面后重新阅读")
	}
	consent := &domain.LedgerConsent{
		OwnerRef: owner, PolicyVersion: domain.LedgerConsentPolicyVersion,
		IPHash: s.keys.HashIP(ip), ConsentedAt: s.now().UTC(),
	}
	if err := s.repo.CreateConsent(ctx, consent); err != nil {
		return nil, err
	}
	if err := s.repo.AppendAudit(ctx, owner, "privacy.consent", "consent", domain.LedgerConsentPolicyVersion, traceID); err != nil {
		return nil, err
	}
	at := consent.ConsentedAt
	return &ConsentView{
		PolicyVersion: domain.LedgerConsentPolicyVersion, Required: false, ConsentedAt: &at,
	}, nil
}

// ExportPayload 全量导出载荷（个人信息可携带权）。
type ExportPayload struct {
	ExportedAt    time.Time             `json:"exported_at"`
	PolicyVersion string                `json:"policy_version"`
	Notice        string                `json:"notice"`
	Items         []ItemView            `json:"items"`
	Members       []MemberView          `json:"members"`
	Snapshots     []SnapshotView        `json:"snapshots"`
	Consent       *domain.LedgerConsent `json:"consent,omitempty"`
	Audit         []ExportAuditEntry    `json:"audit"`
}

// ExportAuditEntry 导出时的审计摘要（不含金额）。
type ExportAuditEntry struct {
	Action    string    `json:"action"`
	Target    string    `json:"target"`
	CreatedAt time.Time `json:"created_at"`
}

// Export 导出该用户全部账本数据（JSON），并记录审计事件。
func (s *Service) Export(ctx context.Context, owner, traceID string) ([]byte, string, error) {
	if err := s.ensureConsent(ctx, s.repo, owner); err != nil {
		return nil, "", err
	}
	views, err := s.loadItems(ctx, s.repo, owner)
	if err != nil {
		return nil, "", err
	}
	members, err := s.ListMembers(ctx, owner)
	if err != nil {
		return nil, "", err
	}
	snapshots, err := s.Snapshots(ctx, owner, s.now().UTC().Format("2006"))
	if err != nil {
		return nil, "", err
	}
	consent, err := s.repo.GetConsent(ctx, owner, domain.LedgerConsentPolicyVersion)
	if err != nil {
		return nil, "", err
	}
	logs, err := s.repo.ListAuditLogs(ctx, owner)
	if err != nil {
		return nil, "", err
	}
	audit := make([]ExportAuditEntry, 0, len(logs))
	for _, l := range logs {
		audit = append(audit, ExportAuditEntry{Action: l.Action, Target: l.TargetType + ":" + l.TargetID, CreatedAt: l.CreatedAt})
	}
	payload := ExportPayload{
		ExportedAt: s.now().UTC(), PolicyVersion: domain.LedgerConsentPolicyVersion,
		Notice: "本文件含家庭资产明细，请妥善保管；金额按录入币种面值计入，不含汇率换算。",
		Items:  views, Members: members, Snapshots: snapshots, Consent: consent, Audit: audit,
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, "", err
	}
	if err := s.repo.AppendAudit(ctx, owner, "privacy.export", "owner", owner[:8], traceID); err != nil {
		return nil, "", err
	}
	// 文件名只含日期，避免经由文件名泄露任何资产信息。
	filename := fmt.Sprintf("家庭资产账本-导出-%s.json", s.now().UTC().Format("20060102"))
	return raw, filename, nil
}

// DeletionView 注销申请状态。
type DeletionView struct {
	Status       string     `json:"status"`
	RequestedAt  *time.Time `json:"requested_at"`
	ExecuteAfter *time.Time `json:"execute_after"`
	DaysLeft     int        `json:"days_left"`
}

// DeletionStatus 查询注销状态（已过期未清扫时按 days_left=0 返回）。
func (s *Service) DeletionStatus(ctx context.Context, owner string) (*DeletionView, error) {
	req, err := s.repo.GetDeletion(ctx, owner)
	if err != nil {
		return nil, err
	}
	if req == nil || req.Status != domain.LedgerDeletionPending {
		return &DeletionView{Status: "none"}, nil
	}
	days := int(time.Until(req.ExecuteAfter).Hours() / 24)
	if days < 0 {
		days = 0
	}
	requested, after := req.RequestedAt, req.ExecuteAfter
	return &DeletionView{
		Status: req.Status, RequestedAt: &requested, ExecuteAfter: &after, DaysLeft: days,
	}, nil
}

// RequestDeletion 申请注销：30 天冷静期内可撤销，到期执行密码学删除。
func (s *Service) RequestDeletion(ctx context.Context, owner, traceID string) (*DeletionView, error) {
	now := s.now().UTC()
	req := &domain.LedgerDeletionRequest{
		OwnerRef: owner, Status: domain.LedgerDeletionPending,
		RequestedAt: now, ExecuteAfter: now.AddDate(0, 0, s.opts.DeletionGraceDays), UpdatedAt: now,
	}
	if err := s.repo.UpsertDeletion(ctx, req); err != nil {
		return nil, err
	}
	if err := s.repo.AppendAudit(ctx, owner, "privacy.deletion.request", "owner", owner[:8], traceID); err != nil {
		return nil, err
	}
	requested, after := req.RequestedAt, req.ExecuteAfter
	return &DeletionView{
		Status: req.Status, RequestedAt: &requested, ExecuteAfter: &after,
		DaysLeft: s.opts.DeletionGraceDays,
	}, nil
}

// CancelDeletion 撤销注销（冷静期内）。
func (s *Service) CancelDeletion(ctx context.Context, owner, traceID string) (*DeletionView, error) {
	req, err := s.repo.GetDeletion(ctx, owner)
	if err != nil {
		return nil, err
	}
	if req == nil || req.Status != domain.LedgerDeletionPending {
		return &DeletionView{Status: "none"}, nil
	}
	req.Status = domain.LedgerDeletionCancelled
	req.UpdatedAt = s.now().UTC()
	if err := s.repo.UpsertDeletion(ctx, req); err != nil {
		return nil, err
	}
	if err := s.repo.AppendAudit(ctx, owner, "privacy.deletion.cancel", "owner", owner[:8], traceID); err != nil {
		return nil, err
	}
	return &DeletionView{Status: req.Status}, nil
}

// PurgeNow 立即执行密码学删除（需新鲜令牌）。
func (s *Service) PurgeNow(ctx context.Context, owner, traceID string) error {
	if err := s.repo.AppendAudit(ctx, owner, "privacy.deletion.purge", "owner", owner[:8], traceID); err != nil {
		return err
	}
	return s.repo.PurgeOwner(ctx, owner)
}

// PurgeDue 清扫已到期注销（供定时任务调用），返回处理数量。
func (s *Service) PurgeDue(ctx context.Context) (int, error) {
	reqs, err := s.repo.ListDueDeletions(ctx, s.now().UTC(), 50)
	if err != nil {
		return 0, err
	}
	purged := 0
	for _, req := range reqs {
		if err := s.repo.PurgeOwner(ctx, req.OwnerRef); err != nil {
			continue
		}
		purged++
	}
	return purged, nil
}
