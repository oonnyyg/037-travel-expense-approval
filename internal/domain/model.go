package domain

// 费用单据状态。
const (
	StatusDraft          = "draft"
	StatusPendingManager = "pending_manager"
	StatusPendingFinance = "pending_finance"
	StatusApproved       = "approved"
	StatusRejected       = "rejected"
)

// 允许的费用类别。
var categories = map[string]bool{
	"交通": true,
	"住宿": true,
	"餐饮": true,
}

// LineItem 是一条费用明细，金额单位为分（正整数）。
type LineItem struct {
	Category    string `json:"category"`
	InvoiceNo   string `json:"invoice_no"`
	Desc        string `json:"desc"`
	AmountCents int64  `json:"amount_cents"`
}

// HistoryEntry 记录一次成功的提交或审批操作。
type HistoryEntry struct {
	Action     string `json:"action"`
	ActorID    string `json:"actor_id,omitempty"`
	TotalCents int64  `json:"total_cents,omitempty"`
	Reason     string `json:"reason,omitempty"`
	UnixNano   int64  `json:"unix_nano"`
}

// Report 是一张差旅报销单。
type Report struct {
	ID          string         `json:"id"`
	ApplicantID string         `json:"applicant_id"`
	ManagerID   string         `json:"manager_id"`
	FinanceID   string         `json:"finance_id"`
	Status      string         `json:"status"`
	Items       []LineItem     `json:"items"`
	History     []HistoryEntry `json:"history,omitempty"`
}

// TotalCents 返回明细金额之和。
func (r *Report) TotalCents() int64 {
	var total int64
	for _, item := range r.Items {
		total += item.AmountCents
	}
	return total
}

// ValidateLineItem 校验单条明细。
func ValidateLineItem(item LineItem) error {
	if !categories[item.Category] {
		return ErrBadCategory
	}
	if item.InvoiceNo == "" {
		return ErrEmptyInvoice
	}
	if item.Desc == "" {
		return ErrEmptyDesc
	}
	if item.AmountCents <= 0 {
		return ErrBadAmount
	}
	return nil
}

// ValidatePeople 校验申请人、主管、财务三人编号非空且互不相同。
func ValidatePeople(applicant, manager, finance string) error {
	if applicant == "" || manager == "" || finance == "" {
		return ErrEmptyPerson
	}
	if applicant == manager || applicant == finance || manager == finance {
		return ErrDistinctPeople
	}
	return nil
}
