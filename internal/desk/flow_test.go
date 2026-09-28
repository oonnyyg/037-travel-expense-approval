package desk

import (
	"path/filepath"
	"testing"
)

func setupDesk(t *testing.T, dir string) *Desk {
	t.Helper()
	d := New(dir)
	if r := d.Handle(Request{Op: "init", Threshold: 1000}); !r.OK {
		t.Fatalf("init: %+v", r)
	}
	return d
}

func makeSheet(t *testing.T, d *Desk, applicant string, items ...Item) string {
	t.Helper()
	r := d.Handle(Request{Op: "create", Applicant: applicant, Manager: "M1", Finance: "F1", Items: items})
	if !r.OK {
		t.Fatalf("create: %+v", r)
	}
	return r.ID
}

func TestDraftDoesNotOccupyAndReplaceRules(t *testing.T) {
	d := setupDesk(t, "")
	// 两张草稿使用同一发票：草稿不占用，允许。
	id1 := makeSheet(t, d, "A1", item("I1", 100))
	id2 := makeSheet(t, d, "A2", item("I1", 200))

	// 非草稿不能替换明细。
	if r := d.Handle(Request{Op: "submit", ID: id1}); !r.OK {
		t.Fatalf("submit id1: %+v", r)
	}
	if r := d.Handle(Request{Op: "replace_items", ID: id1, Items: []Item{item("I9", 5)}}); r.Error != "not_editable" {
		t.Fatalf("已提交单据替换明细应拒绝, got %+v", r)
	}
	// id2 提交时整单冲突，保持草稿、不部分占用。
	r := d.Handle(Request{Op: "submit", ID: id2})
	if r.Error != "invoice_already_occupied" {
		t.Fatalf("发票冲突应整单拒绝, got %+v", r)
	}
	g := d.Handle(Request{Op: "get", ID: id2}).Sheet
	if g.Status != StatusDraft {
		t.Fatalf("冲突单应保持草稿, got %s", g.Status)
	}

	// 草稿可替换明细；替换后总额重新计算。
	r = d.Handle(Request{Op: "replace_items", ID: id2,
		Items: []Item{item("I2", 300), {Category: CatLodging, Invoice: "I3", Desc: "酒店", Amount: 400}}})
	if !r.OK || r.Total != 700 {
		t.Fatalf("替换明细失败: %+v", r)
	}
	// 空明细的草稿不能提交。
	id3 := makeSheet(t, d, "A3")
	if r = d.Handle(Request{Op: "submit", ID: id3}); r.Error != "empty_items" {
		t.Fatalf("无明细提交应拒绝, got %+v", r)
	}
}

func TestCrossSheetConflictAfterApproval(t *testing.T) {
	d := setupDesk(t, "")
	id1 := makeSheet(t, d, "A1", item("I1", 500))
	d.Handle(Request{Op: "submit", ID: id1})
	d.Handle(Request{Op: "approve", ID: id1, Reviewer: "M1"}) // 500 < 1000 直接批准

	id2 := makeSheet(t, d, "A2", item("I1", 100), item("I2", 100))
	r := d.Handle(Request{Op: "submit", ID: id2})
	if r.Error != "invoice_already_occupied" {
		t.Fatalf("已批准单据的发票应继续占用, got %+v", r)
	}
}

func TestRejectReleasesInvoicesAndCannotResubmit(t *testing.T) {
	d := setupDesk(t, "")
	id1 := makeSheet(t, d, "A1", item("I1", 5000))
	d.Handle(Request{Op: "submit", ID: id1})
	// 主管通过 -> 财务拒绝。
	d.Handle(Request{Op: "approve", ID: id1, Reviewer: "M1"})
	r := d.Handle(Request{Op: "reject", ID: id1, Reviewer: "F1", Reason: "发票信息有误"})
	if !r.OK || r.Status != StatusRejected {
		t.Fatalf("财务拒绝失败: %+v", r)
	}
	// 已拒绝不可再提交。
	if r = d.Handle(Request{Op: "submit", ID: id1}); r.Error != "not_submittable" {
		t.Fatalf("已拒绝单据不可再提交, got %+v", r)
	}
	// 发票已释放，新单据可以复用同号。
	id2 := makeSheet(t, d, "A2", item("I1", 100))
	if r = d.Handle(Request{Op: "submit", ID: id2}); !r.OK {
		t.Fatalf("拒绝后发票应可复用, got %+v", r)
	}
	// 历史完整记录提交与两级审批。
	g := d.Handle(Request{Op: "get", ID: id1}).Sheet
	actions := ""
	for _, h := range g.History {
		actions += h.Action + "|"
	}
	want := "submit|manager_approve|finance_reject|"
	if actions != want {
		t.Fatalf("历史记录不符: got %s want %s", actions, want)
	}
	if g.CurrentReviewer != "" {
		t.Fatalf("已结束单据不应有当前审批人")
	}
}

func TestListFilterAndSort(t *testing.T) {
	d := setupDesk(t, "")
	id1 := makeSheet(t, d, "A1", item("I1", 100)) // 草稿
	id2 := makeSheet(t, d, "A1", item("I2", 100))
	d.Handle(Request{Op: "submit", ID: id2}) // pending_manager
	id3 := makeSheet(t, d, "A2", item("I3", 100))
	d.Handle(Request{Op: "submit", ID: id3})
	d.Handle(Request{Op: "approve", ID: id3, Reviewer: "M1"}) // approved

	r := d.Handle(Request{Op: "list", Applicant: "A1"})
	if len(r.Sheets) != 2 || r.Sheets[0].ID != id1 || r.Sheets[1].ID != id2 {
		t.Fatalf("按申请人筛选/排序错误: %+v", r.Sheets)
	}
	r = d.Handle(Request{Op: "list", Applicant: "A1", Status: StatusApproved})
	if len(r.Sheets) != 0 {
		t.Fatalf("组合筛选应为空: %+v", r.Sheets)
	}
	r = d.Handle(Request{Op: "list", Status: StatusApproved})
	if len(r.Sheets) != 1 || r.Sheets[0].ID != id3 {
		t.Fatalf("按状态筛选错误: %+v", r.Sheets)
	}
}

func TestPersistenceReload(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	d := setupDesk(t, dir)
	id1 := makeSheet(t, d, "A1", item("I1", 5000))
	d.Handle(Request{Op: "submit", ID: id1})
	d.Handle(Request{Op: "approve", ID: id1, Reviewer: "M1"}) // pending_finance
	id2 := makeSheet(t, d, "A2", item("I2", 100))             // 草稿

	d2, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if d2.Threshold() != 1000 {
		t.Fatalf("门槛未恢复")
	}
	g := d2.Handle(Request{Op: "get", ID: id1}).Sheet
	if g == nil || g.Status != StatusPendingFinance || g.CurrentReviewer != "F1" || g.Total != 5000 {
		t.Fatalf("待审批单状态未恢复: %+v", g)
	}
	if len(g.History) != 2 {
		t.Fatalf("历史未恢复: %+v", g.History)
	}
	// 占用关系恢复：同号发票提交被拒。
	id3 := d2.Handle(createReq(item("I1", 10))).ID
	if r := d2.Handle(Request{Op: "submit", ID: id3}); r.Error != "invoice_already_occupied" {
		t.Fatalf("重开后发票占用应保持, got %+v", r)
	}
	// 计数器恢复：新单号继续递增。
	id4 := d2.Handle(createReq(item("I4", 1))).ID
	if id4 <= id2 {
		t.Fatalf("单号计数器未恢复, new=%s after=%s", id4, id2)
	}
	// 重新初始化被拒绝。
	if r := d2.Handle(Request{Op: "init", Threshold: 1}); r.Error != "already_initialized" {
		t.Fatalf("重开后重复 init 应拒绝, got %+v", r)
	}
}
