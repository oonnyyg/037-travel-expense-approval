package domain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"
)

const stateFileName = "expensedesk-state.json"

// snapshot 是磁盘上的完整状态。
type snapshot struct {
	Initialized bool               `json:"initialized"`
	Threshold   int64              `json:"threshold"`
	NextSeq     int64              `json:"next_seq"`
	Reports     map[string]*Report `json:"reports"`
}

// Service 管理全部报销单，并把状态持久化到数据目录。
type Service struct {
	mu          sync.Mutex
	dir         string
	initialized bool
	threshold   int64
	nextSeq     int64
	reports     map[string]*Report
}

// Open 打开数据目录；目录不存在时创建。
func Open(dir string) (*Service, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &Service{dir: dir, reports: map[string]*Report{}}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Service) load() error {
	data, err := os.ReadFile(filepath.Join(s.dir, stateFileName))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var snap snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	s.initialized = snap.Initialized
	s.threshold = snap.Threshold
	s.nextSeq = snap.NextSeq
	if snap.Reports != nil {
		s.reports = snap.Reports
	}
	return nil
}

// persist 在调用方已持锁的前提下原子写入状态。
func (s *Service) persistLocked() error {
	snap := snapshot{
		Initialized: s.initialized,
		Threshold:   s.threshold,
		NextSeq:     s.nextSeq,
		Reports:     s.reports,
	}
	data, err := json.MarshalIndent(&snap, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(s.dir, stateFileName+".tmp")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(s.dir, stateFileName))
}

// Init 设置二审金额门槛，只能设置一次，且必须为正整数。
func (s *Service) Init(threshold int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.initialized {
		return ErrAlreadyInit
	}
	if threshold <= 0 {
		return ErrBadThreshold
	}
	s.initialized = true
	s.threshold = threshold
	s.nextSeq = 1
	return s.persistLocked()
}

// Threshold 返回门槛及是否已初始化。
func (s *Service) Threshold() (int64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.threshold, s.initialized
}

func (s *Service) newIDLocked() string {
	id := "R" + strconv.FormatInt(s.nextSeq, 10)
	s.nextSeq++
	return id
}

// CreateDraft 创建一张草稿单。
func (s *Service) CreateDraft(applicant, manager, finance string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.initialized {
		return "", ErrNotInit
	}
	if err := ValidatePeople(applicant, manager, finance); err != nil {
		return "", err
	}
	id := s.newIDLocked()
	s.reports[id] = &Report{
		ID:          id,
		ApplicantID: applicant,
		ManagerID:   manager,
		FinanceID:   finance,
		Status:      StatusDraft,
		Items:       []LineItem{},
	}
	if err := s.persistLocked(); err != nil {
		return "", err
	}
	return id, nil
}

func validateItems(items []LineItem) error {
	seen := map[string]bool{}
	for _, item := range items {
		if err := ValidateLineItem(item); err != nil {
			return err
		}
		if seen[item.InvoiceNo] {
			return ErrDuplicateInvoice
		}
		seen[item.InvoiceNo] = true
	}
	return nil
}

// ReplaceItems 仅在草稿状态下整体替换明细。
func (s *Service) ReplaceItems(id string, items []LineItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	report, ok := s.reports[id]
	if !ok {
		return ErrReportNotFound
	}
	if report.Status != StatusDraft {
		return ErrNotDraft
	}
	if items == nil {
		items = []LineItem{}
	}
	if err := validateItems(items); err != nil {
		return err
	}
	report.Items = items
	return s.persistLocked()
}

// invoiceOwnersLocked 重建当前占用发票的单号映射（待审批与已批准）。
func (s *Service) invoiceOwnersLocked() map[string]string {
	owners := map[string]string{}
	for _, r := range s.reports {
		if r.Status == StatusPendingManager || r.Status == StatusPendingFinance || r.Status == StatusApproved {
			for _, item := range r.Items {
				owners[item.InvoiceNo] = r.ID
			}
		}
	}
	return owners
}

// Submit 提交草稿；任一发票被其他活动单据占用则整单拒绝。
func (s *Service) Submit(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	report, ok := s.reports[id]
	if !ok {
		return ErrReportNotFound
	}
	if report.Status != StatusDraft {
		return ErrNotSubmittable
	}
	if len(report.Items) == 0 {
		return ErrEmptyItems
	}
	owners := s.invoiceOwnersLocked()
	for _, item := range report.Items {
		if owner, conflict := owners[item.InvoiceNo]; conflict && owner != id {
			return ErrInvoiceConflict
		}
	}
	report.Status = StatusPendingManager
	report.History = append(report.History, HistoryEntry{
		Action:     "submit",
		TotalCents: report.TotalCents(),
		UnixNano:   time.Now().UnixNano(),
	})
	return s.persistLocked()
}

// Approve 记录当前阶段审批人的通过决定。
func (s *Service) Approve(id, actorID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	report, ok := s.reports[id]
	if !ok {
		return ErrReportNotFound
	}
	switch report.Status {
	case StatusPendingManager:
		if actorID != report.ManagerID {
			return ErrWrongStage
		}
		if report.TotalCents() >= s.threshold {
			report.Status = StatusPendingFinance
		} else {
			report.Status = StatusApproved
		}
	case StatusPendingFinance:
		if actorID != report.FinanceID {
			return ErrWrongStage
		}
		report.Status = StatusApproved
	default:
		if report.Status == StatusApproved || report.Status == StatusRejected {
			return ErrReportEnded
		}
		return ErrWrongStage
	}
	report.History = append(report.History, HistoryEntry{
		Action:   "approve",
		ActorID:  actorID,
		UnixNano: time.Now().UnixNano(),
	})
	return s.persistLocked()
}

// Reject 记录当前阶段审批人的拒绝决定，必须填写原因，并释放全部发票。
func (s *Service) Reject(id, actorID, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	report, ok := s.reports[id]
	if !ok {
		return ErrReportNotFound
	}
	if reason == "" {
		return ErrEmptyReason
	}
	switch report.Status {
	case StatusPendingManager:
		if actorID != report.ManagerID {
			return ErrWrongStage
		}
	case StatusPendingFinance:
		if actorID != report.FinanceID {
			return ErrWrongStage
		}
	default:
		if report.Status == StatusApproved || report.Status == StatusRejected {
			return ErrReportEnded
		}
		return ErrWrongStage
	}
	report.Status = StatusRejected
	report.History = append(report.History, HistoryEntry{
		Action:   "reject",
		ActorID:  actorID,
		Reason:   reason,
		UnixNano: time.Now().UnixNano(),
	})
	return s.persistLocked()
}

// ReportView 是查询返回的单据视图。
type ReportView struct {
	ID              string         `json:"id"`
	ApplicantID     string         `json:"applicant_id"`
	ManagerID       string         `json:"manager_id"`
	FinanceID       string         `json:"finance_id"`
	Status          string         `json:"status"`
	Items           []LineItem     `json:"items"`
	TotalCents      int64          `json:"total_cents"`
	CurrentApprover string         `json:"current_approver,omitempty"`
	History         []HistoryEntry `json:"history,omitempty"`
}

func toView(r *Report) ReportView {
	v := ReportView{
		ID:          r.ID,
		ApplicantID: r.ApplicantID,
		ManagerID:   r.ManagerID,
		FinanceID:   r.FinanceID,
		Status:      r.Status,
		Items:       r.Items,
		TotalCents:  r.TotalCents(),
		History:     r.History,
	}
	switch r.Status {
	case StatusPendingManager:
		v.CurrentApprover = r.ManagerID
	case StatusPendingFinance:
		v.CurrentApprover = r.FinanceID
	}
	return v
}

// Get 查询单张单据。
func (s *Service) Get(id string) (ReportView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	report, ok := s.reports[id]
	if !ok {
		return ReportView{}, ErrReportNotFound
	}
	return toView(report), nil
}

// List 按申请人和状态组合筛选，空字符串表示不过滤；按单号排序。
func (s *Service) List(applicantID, status string) ([]ReportView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.reports))
	for id := range s.reports {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	views := []ReportView{}
	for _, id := range ids {
		r := s.reports[id]
		if applicantID != "" && r.ApplicantID != applicantID {
			continue
		}
		if status != "" && r.Status != status {
			continue
		}
		views = append(views, toView(r))
	}
	return views, nil
}
