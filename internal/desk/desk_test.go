package desk

import "testing"

func item(inv string, amount int64) Item {
	return Item{Category: CatTransport, Invoice: inv, Desc: "出租车", Amount: amount}
}

func createReq(items ...Item) Request {
	return Request{Op: "create", Applicant: "A1", Manager: "M1", Finance: "F1", Items: items}
}

func TestInitRules(t *testing.T) {
	d := New("")
	if r := d.Handle(Request{Op: "create", Applicant: "A", Manager: "M", Finance: "F"}); r.Error != "not_initialized" {
		t.Fatalf("未初始化应拒绝创建, got %+v", r)
	}
	if r := d.Handle(Request{Op: "init", Threshold: 0}); r.Error != "invalid_threshold" {
		t.Fatalf("门槛 0 应拒绝, got %+v", r)
	}
	if r := d.Handle(Request{Op: "init", Threshold: -3}); r.Error != "invalid_threshold" {
		t.Fatalf("负数门槛应拒绝, got %+v", r)
	}
	if r := d.Handle(Request{Op: "init", Threshold: 1000}); !r.OK {
		t.Fatalf("初始化失败: %+v", r)
	}
	if r := d.Handle(Request{Op: "init", Threshold: 2000}); r.Error != "already_initialized" {
		t.Fatalf("重复初始化应拒绝, got %+v", r)
	}
}

func TestCreateValidation(t *testing.T) {
	d := New("")
	d.Handle(Request{Op: "init", Threshold: 1000})

	r := d.Handle(Request{Op: "create", Applicant: "A", Manager: "M", Finance: "M"})
	if r.Error != "invalid_people" {
		t.Fatalf("编号相同应拒绝, got %+v", r)
	}
	r = d.Handle(Request{Op: "create", Applicant: " ", Manager: "M", Finance: "F"})
	if r.Error != "invalid_people" {
		t.Fatalf("空编号应拒绝, got %+v", r)
	}

	bad := Item{Category: "零食", Invoice: "I1", Desc: "x", Amount: 10}
	if r = d.Handle(createReq(bad)); r.Error != "invalid_category" {
		t.Fatalf("非法类别应拒绝, got %+v", r)
	}
	bad.Category = CatDining
	bad.Amount = 0
	if r = d.Handle(createReq(bad)); r.Error != "invalid_amount" {
		t.Fatalf("零金额应拒绝, got %+v", r)
	}
	bad.Amount = 10
	bad.Invoice = "  "
	if r = d.Handle(createReq(bad)); r.Error != "invalid_invoice" {
		t.Fatalf("空发票号应拒绝, got %+v", r)
	}
	bad.Invoice = "I1"
	bad.Desc = ""
	if r = d.Handle(createReq(bad)); r.Error != "invalid_desc" {
		t.Fatalf("空说明应拒绝, got %+v", r)
	}
	if r = d.Handle(createReq(item("I1", 100), item("I1", 50))); r.Error != "duplicate_invoice_in_sheet" {
		t.Fatalf("单内发票重复应拒绝, got %+v", r)
	}
}

// 总额严格小于门槛：主管通过即批准。
func TestThresholdBelow(t *testing.T) {
	d := New("")
	d.Handle(Request{Op: "init", Threshold: 1000})
	c := d.Handle(createReq(item("I1", 999)))
	id := c.ID
	d.Handle(Request{Op: "submit", ID: id})

	if r := d.Handle(Request{Op: "approve", ID: id, Reviewer: "F1"}); r.Error != "reviewer_forbidden" {
		t.Fatalf("财务越级审批应拒绝, got %+v", r)
	}
	r := d.Handle(Request{Op: "approve", ID: id, Reviewer: "M1"})
	if !r.OK || r.Status != StatusApproved {
		t.Fatalf("主管通过后应直接批准, got %+v", r)
	}
}

// 总额等于门槛：需要财务二审；超过门槛同理。
func TestThresholdEqualAndAbove(t *testing.T) {
	for _, amount := range []int64{1000, 1001} {
		d := New("")
		d.Handle(Request{Op: "init", Threshold: 1000})
		c := d.Handle(createReq(item("I1", amount)))
		id := c.ID
		d.Handle(Request{Op: "submit", ID: id})

		r := d.Handle(Request{Op: "approve", ID: id, Reviewer: "M1"})
		if r.Status != StatusPendingFinance || r.Current != "F1" {
			t.Fatalf("amount=%d 主管通过后应等待财务, got %+v", amount, r)
		}
		// 主管重复操作 / 再次越级都拒绝。
		if r = d.Handle(Request{Op: "approve", ID: id, Reviewer: "M1"}); r.Error != "reviewer_forbidden" {
			t.Fatalf("主管重复审批应拒绝, got %+v", r)
		}
		if r = d.Handle(Request{Op: "reject", ID: id, Reviewer: "F1"}); r.Error != "reason_required" {
			t.Fatalf("财务无理由拒绝应拒绝, got %+v", r)
		}
		r = d.Handle(Request{Op: "approve", ID: id, Reviewer: "F1"})
		if !r.OK || r.Status != StatusApproved {
			t.Fatalf("财务通过后应批准, got %+v", r)
		}
		if r = d.Handle(Request{Op: "approve", ID: id, Reviewer: "F1"}); r.Error != "already_finished" {
			t.Fatalf("已结束单据继续审批应拒绝, got %+v", r)
		}
	}
}
