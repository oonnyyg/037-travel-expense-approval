package desk

import "encoding/json"

// 费用类别。
const (
	CatTransport = "交通"
	CatLodging   = "住宿"
	CatDining    = "餐饮"
)

// 单据状态。
const (
	StatusDraft          = "draft"
	StatusPendingManager = "pending_manager"
	StatusPendingFinance = "pending_finance"
	StatusApproved       = "approved"
	StatusRejected       = "rejected"
)

// Item 是一条费用明细，金额单位为分，必须为正整数。
type Item struct {
	Category string `json:"category"`
	Invoice  string `json:"invoice"`
	Desc     string `json:"desc"`
	Amount   int64  `json:"amount"`
}

// HistoryEntry 记录一次成功的提交或审批操作。
type HistoryEntry struct {
	Action   string `json:"action"` // submit / manager_approve / finance_approve / manager_reject / finance_reject
	Actor    string `json:"actor"`
	Reason   string `json:"reason,omitempty"`
	UnixNano int64  `json:"unix_nano"`
}

// Sheet 是一张报销单的完整状态。
type Sheet struct {
	ID        string         `json:"id"`
	Applicant string         `json:"applicant"`
	Manager   string         `json:"manager"`
	Finance   string         `json:"finance"`
	Status    string         `json:"status"`
	Items     []Item         `json:"items"`
	Total     int64          `json:"total"`
	History   []HistoryEntry `json:"history"`
}

// CurrentReviewer 返回当前阶段有权决定的审批人；草稿和已结束单据返回空串。
func (s *Sheet) CurrentReviewer() string {
	switch s.Status {
	case StatusPendingManager:
		return s.Manager
	case StatusPendingFinance:
		return s.Finance
	default:
		return ""
	}
}

// Request 是逐行读入的 JSON 请求，Op 决定其余字段。
type Request struct {
	Op        string `json:"op"`
	Threshold int64  `json:"threshold"`
	ID        string `json:"id"`
	Applicant string `json:"applicant"`
	Manager   string `json:"manager"`
	Finance   string `json:"finance"`
	Items     []Item `json:"items"`
	Reviewer  string `json:"reviewer"`
	Reason    string `json:"reason"`
	Status    string `json:"status"`
}

// Response 是统一的 JSON 返回。
type Response struct {
	OK      bool           `json:"ok"`
	Error   string         `json:"error,omitempty"`
	Message string         `json:"message,omitempty"`
	ID      string         `json:"id,omitempty"`
	Status  string         `json:"status,omitempty"`
	Total   int64          `json:"total,omitempty"`
	Current string         `json:"current_reviewer,omitempty"`
	Sheet   *SheetDetail   `json:"sheet,omitempty"`
	Sheets  []SheetSummary `json:"sheets,omitempty"`
}

// SheetDetail 是详情返回结构。
type SheetDetail struct {
	ID              string         `json:"id"`
	Applicant       string         `json:"applicant"`
	Manager         string         `json:"manager"`
	Finance         string         `json:"finance"`
	Status          string         `json:"status"`
	Items           []Item         `json:"items"`
	Total           int64          `json:"total"`
	CurrentReviewer string         `json:"current_reviewer,omitempty"`
	History         []HistoryEntry `json:"history"`
}

// SheetSummary 是列表项结构。
type SheetSummary struct {
	ID        string `json:"id"`
	Applicant string `json:"applicant"`
	Status    string `json:"status"`
	Total     int64  `json:"total"`
}

func detailOf(s *Sheet) *SheetDetail {
	return &SheetDetail{
		ID:              s.ID,
		Applicant:       s.Applicant,
		Manager:         s.Manager,
		Finance:         s.Finance,
		Status:          s.Status,
		Items:           append([]Item(nil), s.Items...),
		Total:           s.Total,
		CurrentReviewer: s.CurrentReviewer(),
		History:         append([]HistoryEntry(nil), s.History...),
	}
}

func errResp(code, msg string) Response {
	return Response{OK: false, Error: code, Message: msg}
}

var _ = json.Marshal
