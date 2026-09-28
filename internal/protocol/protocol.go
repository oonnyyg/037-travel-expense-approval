package protocol

import (
	"encoding/json"
	"errors"

	"example.com/travel-expense-approval/internal/domain"
)

// Request 是逐行 JSON 请求。action 决定其余字段。
type Request struct {
	Action      string            `json:"action"`
	Threshold   *int64            `json:"threshold,omitempty"`
	ID          string            `json:"id,omitempty"`
	ApplicantID string            `json:"applicant_id,omitempty"`
	ManagerID   string            `json:"manager_id,omitempty"`
	FinanceID   string            `json:"finance_id,omitempty"`
	Items       []domain.LineItem `json:"items,omitempty"`
	ActorID     string            `json:"actor_id,omitempty"`
	Reason      string            `json:"reason,omitempty"`
	Status      string            `json:"status,omitempty"`
}

// Response 是每个请求对应的单行 JSON 结果。
type Response struct {
	OK          bool                `json:"ok"`
	Error       string              `json:"error,omitempty"`
	ID          string              `json:"id,omitempty"`
	Threshold   int64               `json:"threshold,omitempty"`
	Initialized bool                `json:"initialized,omitempty"`
	Report      *domain.ReportView  `json:"report,omitempty"`
	Reports     []domain.ReportView `json:"reports,omitempty"`
}

var errorCodes = map[error]string{
	domain.ErrBadThreshold:     "bad_threshold",
	domain.ErrAlreadyInit:      "already_initialized",
	domain.ErrNotInit:          "not_initialized",
	domain.ErrEmptyPerson:      "empty_person_id",
	domain.ErrDistinctPeople:   "people_must_be_distinct",
	domain.ErrBadCategory:      "bad_category",
	domain.ErrEmptyInvoice:     "empty_invoice_no",
	domain.ErrEmptyDesc:        "empty_description",
	domain.ErrBadAmount:        "bad_amount",
	domain.ErrDuplicateInvoice: "duplicate_invoice_in_report",
	domain.ErrEmptyItems:       "empty_items",
	domain.ErrReportNotFound:   "report_not_found",
	domain.ErrNotDraft:         "not_a_draft",
	domain.ErrNotSubmittable:   "report_not_submittable",
	domain.ErrInvoiceConflict:  "invoice_conflict",
	domain.ErrWrongStage:       "wrong_stage_or_actor",
	domain.ErrReportEnded:      "report_already_ended",
	domain.ErrEmptyReason:      "empty_reject_reason",
	domain.ErrInvalidAction:    "unknown_action",
}

func errorResponse(err error) Response {
	for target, code := range errorCodes {
		if errors.Is(err, target) {
			return Response{OK: false, Error: code}
		}
	}
	return Response{OK: false, Error: "internal_error"}
}

// Handle 处理一个已解析的请求。
func Handle(svc *domain.Service, req Request) Response {
	switch req.Action {
	case "init":
		if req.Threshold == nil {
			return errorResponse(domain.ErrBadThreshold)
		}
		if err := svc.Init(*req.Threshold); err != nil {
			return errorResponse(err)
		}
		return Response{OK: true}
	case "create":
		id, err := svc.CreateDraft(req.ApplicantID, req.ManagerID, req.FinanceID)
		if err != nil {
			return errorResponse(err)
		}
		return Response{OK: true, ID: id}
	case "replace_items":
		if err := svc.ReplaceItems(req.ID, req.Items); err != nil {
			return errorResponse(err)
		}
		return Response{OK: true}
	case "submit":
		if err := svc.Submit(req.ID); err != nil {
			return errorResponse(err)
		}
		return Response{OK: true}
	case "approve":
		if err := svc.Approve(req.ID, req.ActorID); err != nil {
			return errorResponse(err)
		}
		return Response{OK: true}
	case "reject":
		if err := svc.Reject(req.ID, req.ActorID, req.Reason); err != nil {
			return errorResponse(err)
		}
		return Response{OK: true}
	case "get":
		view, err := svc.Get(req.ID)
		if err != nil {
			return errorResponse(err)
		}
		return Response{OK: true, Report: &view}
	case "list":
		views, err := svc.List(req.ApplicantID, req.Status)
		if err != nil {
			return errorResponse(err)
		}
		return Response{OK: true, Reports: views}
	case "status":
		threshold, initialized := svc.Threshold()
		return Response{OK: true, Initialized: initialized, Threshold: threshold}
	default:
		return errorResponse(domain.ErrInvalidAction)
	}
}

// ProcessLine 解析一行 JSON 并执行。
func ProcessLine(svc *domain.Service, line []byte) Response {
	var req Request
	if err := json.Unmarshal(line, &req); err != nil {
		return Response{OK: false, Error: "invalid_json"}
	}
	return Handle(svc, req)
}
