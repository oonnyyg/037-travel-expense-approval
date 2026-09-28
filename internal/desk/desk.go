package desk

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

var validCategories = map[string]bool{
	CatTransport: true,
	CatLodging:   true,
	CatDining:    true,
}

// Desk 持有全部内存状态；并发安全。
type Desk struct {
	mu        sync.Mutex
	dir       string
	threshold int64
	initialed bool
	counter   int64
	sheets    map[string]*Sheet
	// occupied 记录被待审批/已批准单据占用的发票号 -> 单号。
	occupied map[string]string
}

// New 创建内存中的空 Desk（尚未初始化门槛）。
func New(dataDir string) *Desk {
	return &Desk{
		dir:      dataDir,
		sheets:   make(map[string]*Sheet),
		occupied: make(map[string]string),
	}
}

// Threshold 返回二审金额门槛。
func (d *Desk) Threshold() int64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.threshold
}

// Handle 处理一个请求并返回响应；非法请求不会改变任何状态。
func (d *Desk) Handle(req Request) Response {
	d.mu.Lock()
	defer d.mu.Unlock()

	switch req.Op {
	case "init":
		return d.opInit(req)
	case "create":
		return d.opCreate(req)
	case "replace_items":
		return d.opReplaceItems(req)
	case "submit":
		return d.opSubmit(req)
	case "approve":
		return d.opApprove(req, true)
	case "reject":
		return d.opApprove(req, false)
	case "get":
		return d.opGet(req)
	case "list":
		return d.opList(req)
	default:
		return errResp("unknown_op", fmt.Sprintf("未知操作: %s", req.Op))
	}
}

func (d *Desk) requireInit() *Response {
	if !d.initialed {
		r := errResp("not_initialized", "尚未初始化二审金额门槛")
		return &r
	}
	return nil
}

func (d *Desk) opInit(req Request) Response {
	if d.initialed {
		return errResp("already_initialized", fmt.Sprintf("门槛已初始化为 %d，不可修改", d.threshold))
	}
	if req.Threshold <= 0 {
		return errResp("invalid_threshold", "门槛必须为正整数（单位：分）")
	}
	d.threshold = req.Threshold
	d.initialed = true
	if err := d.persistLocked(); err != nil {
		d.threshold = 0
		d.initialed = false
		return errResp("persist_failed", err.Error())
	}
	return Response{OK: true, Message: "初始化成功", Status: "ready"}
}

func nonEmptyTrim(s string) string {
	return strings.TrimSpace(s)
}

func validatePeople(applicant, manager, finance string) (string, string, string, Response) {
	applicant = nonEmptyTrim(applicant)
	manager = nonEmptyTrim(manager)
	finance = nonEmptyTrim(finance)
	if applicant == "" || manager == "" || finance == "" {
		return "", "", "", errResp("invalid_people", "申请人、主管和财务编号均不能为空")
	}
	if applicant == manager || applicant == finance || manager == finance {
		return "", "", "", errResp("invalid_people", "申请人、主管和财务编号必须互不相同")
	}
	return applicant, manager, finance, Response{}
}

func validateItems(items []Item) ([]Item, int64, Response) {
	if len(items) == 0 {
		return nil, 0, errResp("empty_items", "至少需要一项费用")
	}
	clean := make([]Item, 0, len(items))
	seen := make(map[string]bool, len(items))
	var total int64
	for i, it := range items {
		it.Invoice = nonEmptyTrim(it.Invoice)
		it.Desc = nonEmptyTrim(it.Desc)
		if !validCategories[it.Category] {
			return nil, 0, errResp("invalid_category", fmt.Sprintf("第 %d 项类别非法，仅允许交通/住宿/餐饮", i+1))
		}
		if it.Invoice == "" {
			return nil, 0, errResp("invalid_invoice", fmt.Sprintf("第 %d 项发票号不能为空", i+1))
		}
		if it.Desc == "" {
			return nil, 0, errResp("invalid_desc", fmt.Sprintf("第 %d 项说明不能为空", i+1))
		}
		if it.Amount <= 0 {
			return nil, 0, errResp("invalid_amount", fmt.Sprintf("第 %d 项金额必须为正整数（单位：分）", i+1))
		}
		if seen[it.Invoice] {
			return nil, 0, errResp("duplicate_invoice_in_sheet", fmt.Sprintf("发票号 %s 在本单内重复", it.Invoice))
		}
		seen[it.Invoice] = true
		total += it.Amount
		clean = append(clean, it)
	}
	return clean, total, Response{}
}

func (d *Desk) opCreate(req Request) Response {
	if r := d.requireInit(); r != nil {
		return *r
	}
	applicant, manager, finance, bad := validatePeople(req.Applicant, req.Manager, req.Finance)
	if bad.Error != "" {
		return bad
	}
	var items []Item
	var total int64
	if len(req.Items) > 0 {
		var badResp Response
		items, total, badResp = validateItems(req.Items)
		if badResp.Error != "" {
			return badResp
		}
	}
	d.counter++
	id := fmt.Sprintf("S%06d", d.counter)
	s := &Sheet{
		ID:        id,
		Applicant: applicant,
		Manager:   manager,
		Finance:   finance,
		Status:    StatusDraft,
		Items:     items,
		Total:     total,
	}
	d.sheets[id] = s
	if err := d.persistLocked(); err != nil {
		d.counter--
		delete(d.sheets, id)
		return errResp("persist_failed", err.Error())
	}
	return Response{OK: true, ID: id, Status: s.Status, Total: total, Message: "草稿已创建"}
}

func (d *Desk) opReplaceItems(req Request) Response {
	if r := d.requireInit(); r != nil {
		return *r
	}
	s := d.sheets[strings.TrimSpace(req.ID)]
	if s == nil {
		return errResp("sheet_not_found", "单据不存在")
	}
	if s.Status != StatusDraft {
		return errResp("not_editable", "只有草稿单据允许替换明细")
	}
	items, total, bad := validateItems(req.Items)
	if bad.Error != "" {
		return bad
	}
	oldItems, oldTotal := s.Items, s.Total
	s.Items = items
	s.Total = total
	if err := d.persistLocked(); err != nil {
		s.Items, s.Total = oldItems, oldTotal
		return errResp("persist_failed", err.Error())
	}
	return Response{OK: true, ID: s.ID, Status: s.Status, Total: total, Message: "明细已替换"}
}

func (d *Desk) opSubmit(req Request) Response {
	if r := d.requireInit(); r != nil {
		return *r
	}
	s := d.sheets[strings.TrimSpace(req.ID)]
	if s == nil {
		return errResp("sheet_not_found", "单据不存在")
	}
	if s.Status != StatusDraft {
		return errResp("not_submittable", "只有草稿单据可以提交")
	}
	if len(s.Items) == 0 {
		return errResp("empty_items", "提交时至少需要一项费用")
	}
	// 整单校验发票冲突：任一冲突则全部拒绝占用。
	for _, it := range s.Items {
		if holder, ok := d.occupied[it.Invoice]; ok {
			return errResp("invoice_already_occupied",
				fmt.Sprintf("发票号 %s 已被单据 %s 占用，整单拒绝提交", it.Invoice, holder))
		}
	}
	s.Status = StatusPendingManager
	now := time.Now().UnixNano()
	s.History = append(s.History, HistoryEntry{Action: "submit", Actor: s.Applicant, UnixNano: now})
	for _, it := range s.Items {
		d.occupied[it.Invoice] = s.ID
	}
	if err := d.persistLocked(); err != nil {
		s.Status = StatusDraft
		s.History = s.History[:len(s.History)-1]
		for _, it := range s.Items {
			delete(d.occupied, it.Invoice)
		}
		return errResp("persist_failed", err.Error())
	}
	return Response{OK: true, ID: s.ID, Status: s.Status, Total: s.Total, Current: s.CurrentReviewer(), Message: "已提交，等待主管审批"}
}

func (d *Desk) opApprove(req Request, approve bool) Response {
	if r := d.requireInit(); r != nil {
		return *r
	}
	s := d.sheets[strings.TrimSpace(req.ID)]
	if s == nil {
		return errResp("sheet_not_found", "单据不存在")
	}
	reviewer := nonEmptyTrim(req.Reviewer)
	if reviewer == "" {
		return errResp("invalid_reviewer", "审批人编号不能为空")
	}
	var need string
	switch s.Status {
	case StatusDraft:
		return errResp("not_in_review", "单据尚未提交")
	case StatusApproved:
		return errResp("already_finished", "单据已批准，不可继续审批")
	case StatusRejected:
		return errResp("already_finished", "单据已拒绝，不可继续审批")
	case StatusPendingManager:
		need = s.Manager
	case StatusPendingFinance:
		need = s.Finance
	}
	if reviewer != need {
		return errResp("reviewer_forbidden", fmt.Sprintf("当前仅 %s 可以决定，不可越级或越权审批", need))
	}
	if !approve {
		reason := nonEmptyTrim(req.Reason)
		if reason == "" {
			return errResp("reason_required", "拒绝时必须填写原因")
		}
	}

	snapshot := s.snapshotForRollback()
	occ := make([]string, 0, len(s.Items))
	for _, it := range s.Items {
		occ = append(occ, it.Invoice)
	}

	now := time.Now().UnixNano()
	if approve {
		if s.Status == StatusPendingManager {
			if s.Total >= d.threshold {
				s.Status = StatusPendingFinance
				s.History = append(s.History, HistoryEntry{Action: "manager_approve", Actor: reviewer, UnixNano: now})
			} else {
				s.Status = StatusApproved
				s.History = append(s.History, HistoryEntry{Action: "manager_approve", Actor: reviewer, UnixNano: now})
			}
		} else {
			s.Status = StatusApproved
			s.History = append(s.History, HistoryEntry{Action: "finance_approve", Actor: reviewer, UnixNano: now})
		}
	} else {
		action := "manager_reject"
		if s.Status == StatusPendingFinance {
			action = "finance_reject"
		}
		s.Status = StatusRejected
		s.History = append(s.History, HistoryEntry{Action: action, Actor: reviewer, Reason: req.Reason, UnixNano: now})
		for _, inv := range occ {
			delete(d.occupied, inv)
		}
	}

	if err := d.persistLocked(); err != nil {
		s.restoreSnapshot(snapshot)
		if !approve {
			for _, inv := range occ {
				d.occupied[inv] = s.ID
			}
		}
		return errResp("persist_failed", err.Error())
	}

	msg := "已批准"
	if s.Status == StatusPendingFinance {
		msg = "主管已通过，等待财务审批"
	} else if s.Status == StatusRejected {
		msg = "已拒绝并释放全部发票"
	}
	resp := Response{OK: true, ID: s.ID, Status: s.Status, Total: s.Total, Message: msg}
	if s.Status == StatusPendingFinance {
		resp.Current = s.CurrentReviewer()
	}
	return resp
}

func (d *Desk) opGet(req Request) Response {
	if r := d.requireInit(); r != nil {
		return *r
	}
	s := d.sheets[strings.TrimSpace(req.ID)]
	if s == nil {
		return errResp("sheet_not_found", "单据不存在")
	}
	return Response{OK: true, Sheet: detailOf(s)}
}

func (d *Desk) opList(req Request) Response {
	if r := d.requireInit(); r != nil {
		return *r
	}
	applicant := nonEmptyTrim(req.Applicant)
	status := nonEmptyTrim(req.Status)
	if status != "" {
		switch status {
		case StatusDraft, StatusPendingManager, StatusPendingFinance, StatusApproved, StatusRejected:
		default:
			return errResp("invalid_status", "状态筛选值非法")
		}
	}
	out := make([]SheetSummary, 0, len(d.sheets))
	for _, s := range d.sheets {
		if applicant != "" && s.Applicant != applicant {
			continue
		}
		if status != "" && s.Status != status {
			continue
		}
		out = append(out, SheetSummary{ID: s.ID, Applicant: s.Applicant, Status: s.Status, Total: s.Total})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return Response{OK: true, Sheets: out}
}

type sheetSnapshot struct {
	status  string
	items   []Item
	total   int64
	history []HistoryEntry
}

func (s *Sheet) snapshotForRollback() sheetSnapshot {
	return sheetSnapshot{status: s.Status, items: s.Items, total: s.Total, history: s.History}
}

func (s *Sheet) restoreSnapshot(snap sheetSnapshot) {
	s.Status = snap.status
	s.Items = snap.items
	s.Total = snap.total
	s.History = snap.history
}
