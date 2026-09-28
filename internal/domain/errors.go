package domain

import "errors"

// 可区分的业务错误。非法请求返回这些错误，不改变任何状态。
var (
	ErrBadThreshold     = errors.New("threshold must be a positive integer")
	ErrAlreadyInit      = errors.New("state already initialized")
	ErrNotInit          = errors.New("state not initialized")
	ErrEmptyPerson      = errors.New("person id must not be empty")
	ErrDistinctPeople   = errors.New("applicant, manager and finance must be distinct")
	ErrBadCategory      = errors.New("category must be one of 交通/住宿/餐饮")
	ErrEmptyInvoice     = errors.New("invoice number must not be empty")
	ErrEmptyDesc        = errors.New("description must not be empty")
	ErrBadAmount        = errors.New("amount must be a positive integer in cents")
	ErrDuplicateInvoice = errors.New("duplicate invoice within report")
	ErrEmptyItems       = errors.New("report must contain at least one item")
	ErrReportNotFound   = errors.New("report not found")
	ErrNotDraft         = errors.New("only draft reports can be replaced")
	ErrNotSubmittable   = errors.New("only draft reports can be submitted")
	ErrInvoiceConflict  = errors.New("invoice already occupied by another active report")
	ErrWrongStage       = errors.New("actor is not the approver of the current stage")
	ErrReportEnded      = errors.New("report has already ended")
	ErrEmptyReason      = errors.New("rejection reason must not be empty")
	ErrInvalidAction    = errors.New("unknown action")
	ErrInvalidJSON      = errors.New("invalid json request")
)
