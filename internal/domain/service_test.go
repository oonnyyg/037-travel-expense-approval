package domain

import (
	"errors"
	"path/filepath"
	"testing"
)

func newService(t *testing.T, threshold int64) *Service {
	t.Helper()
	svc, err := Open(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Init(threshold); err != nil {
		t.Fatal(err)
	}
	return svc
}

func item(invoice string, amount int64) LineItem {
	return LineItem{Category: "交通", InvoiceNo: invoice, Desc: "taxi", AmountCents: amount}
}

func createReady(t *testing.T, svc *Service, items ...LineItem) string {
	t.Helper()
	id, err := svc.CreateDraft("u1", "m1", "f1")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ReplaceItems(id, items); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestInitThresholdRules(t *testing.T) {
	svc, err := Open(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Init(0); !errors.Is(err, ErrBadThreshold) {
		t.Fatalf("want ErrBadThreshold, got %v", err)
	}
	if err := svc.Init(-5); !errors.Is(err, ErrBadThreshold) {
		t.Fatalf("want ErrBadThreshold, got %v", err)
	}
	if err := svc.Init(1000); err != nil {
		t.Fatal(err)
	}
	if err := svc.Init(2000); !errors.Is(err, ErrAlreadyInit) {
		t.Fatalf("want ErrAlreadyInit, got %v", err)
	}
	if _, err := Open(svc.dir); err != nil {
		t.Fatal(err)
	}
}

func TestThresholdBoundaryApproval(t *testing.T) {
	for _, tc := range []struct {
		name    string
		amount  int64
		approve func(*Service, string) error
		want    string
	}{
		{"below threshold one approver", 999, func(s *Service, id string) error {
			return s.Approve(id, "m1")
		}, StatusApproved},
		{"at threshold needs finance", 1000, func(s *Service, id string) error {
			if err := s.Approve(id, "m1"); err != nil {
				return err
			}
			return s.Approve(id, "f1")
		}, StatusApproved},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := newService(t, 1000)
			id := createReady(t, svc, item("INV-1", tc.amount))
			if err := svc.Submit(id); err != nil {
				t.Fatal(err)
			}
			if err := tc.approve(svc, id); err != nil {
				t.Fatal(err)
			}
			view, _ := svc.Get(id)
			if view.Status != tc.want {
				t.Fatalf("status = %s", view.Status)
			}
		})
	}

	t.Run("at threshold manager alone is not enough", func(t *testing.T) {
		svc := newService(t, 1000)
		id := createReady(t, svc, item("INV-1", 1000))
		if err := svc.Submit(id); err != nil {
			t.Fatal(err)
		}
		if err := svc.Approve(id, "m1"); err != nil {
			t.Fatal(err)
		}
		view, _ := svc.Get(id)
		if view.Status != StatusPendingFinance || view.CurrentApprover != "f1" {
			t.Fatalf("status=%s approver=%s", view.Status, view.CurrentApprover)
		}
	})
}

func TestOutOfOrderAndWrongActorApproval(t *testing.T) {
	svc := newService(t, 1000)
	id := createReady(t, svc, item("INV-1", 1000))

	if err := svc.Approve(id, "m1"); !errors.Is(err, ErrWrongStage) {
		t.Fatalf("approve draft: %v", err)
	}
	if err := svc.Submit(id); err != nil {
		t.Fatal(err)
	}
	if err := svc.Approve(id, "f1"); !errors.Is(err, ErrWrongStage) {
		t.Fatalf("finance skipping manager: %v", err)
	}
	if err := svc.Approve(id, "u1"); !errors.Is(err, ErrWrongStage) {
		t.Fatalf("applicant approving: %v", err)
	}
	if err := svc.Approve(id, "m1"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Approve(id, "m1"); !errors.Is(err, ErrWrongStage) {
		t.Fatalf("manager repeat at finance stage: %v", err)
	}
	if err := svc.Approve(id, "f1"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Approve(id, "f1"); !errors.Is(err, ErrReportEnded) {
		t.Fatalf("approve ended: %v", err)
	}
}

func TestDraftAndItemValidation(t *testing.T) {
	svc := newService(t, 1000)
	id, err := svc.CreateDraft("u1", "u1", "f1")
	if !errors.Is(err, ErrDistinctPeople) {
		t.Fatalf("distinct people: %v", err)
	}
	id, err = svc.CreateDraft("u1", "m1", "f1")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Submit(id); !errors.Is(err, ErrEmptyItems) {
		t.Fatalf("empty submit: %v", err)
	}
	bad := []LineItem{{Category: "餐饮", InvoiceNo: "", Desc: "x", AmountCents: 1}}
	if err := svc.ReplaceItems(id, bad); !errors.Is(err, ErrEmptyInvoice) {
		t.Fatalf("empty invoice: %v", err)
	}
	bad = []LineItem{item("INV-1", 0)}
	if err := svc.ReplaceItems(id, bad); !errors.Is(err, ErrBadAmount) {
		t.Fatalf("zero amount: %v", err)
	}
	if err := svc.ReplaceItems(id, []LineItem{item("INV-1", 100), item("INV-1", 200)}); !errors.Is(err, ErrDuplicateInvoice) {
		t.Fatalf("dup invoice: %v", err)
	}
	if err := svc.ReplaceItems(id, []LineItem{item("INV-1", 100)}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Submit(id); err != nil {
		t.Fatal(err)
	}
	if err := svc.ReplaceItems(id, []LineItem{}); !errors.Is(err, ErrNotDraft) {
		t.Fatalf("replace after submit: %v", err)
	}
}

func TestCrossReportInvoiceConflict(t *testing.T) {
	svc := newService(t, 10000)
	a := createReady(t, svc, item("INV-SHARED", 100), item("INV-A", 50))
	b := createReady(t, svc, item("INV-SHARED", 200), item("INV-B", 60))

	if err := svc.Submit(a); err != nil {
		t.Fatal(err)
	}
	if err := svc.Submit(b); !errors.Is(err, ErrInvoiceConflict) {
		t.Fatalf("conflict submit: %v", err)
	}
	view, _ := svc.Get(b)
	if view.Status != StatusDraft {
		t.Fatalf("rejected submit must keep draft, got %s", view.Status)
	}

	// b 自己未占用任何发票：另一张使用 INV-B 的单据应能提交。
	c := createReady(t, svc, item("INV-B", 60))
	if err := svc.Submit(c); err != nil {
		t.Fatalf("INV-B must not be partially occupied: %v", err)
	}
}

func TestRejectReleasesInvoicesThenReuse(t *testing.T) {
	svc := newService(t, 10000)
	a := createReady(t, svc, item("INV-SHARED", 100))
	b := createReady(t, svc, item("INV-SHARED", 200))
	if err := svc.Submit(a); err != nil {
		t.Fatal(err)
	}
	if err := svc.Submit(b); !errors.Is(err, ErrInvoiceConflict) {
		t.Fatal(err)
	}
	if err := svc.Reject(a, "m1", ""); !errors.Is(err, ErrEmptyReason) {
		t.Fatalf("reason required: %v", err)
	}
	if err := svc.Reject(a, "f1", "wrong stage"); !errors.Is(err, ErrWrongStage) {
		t.Fatalf("finance cannot reject manager stage: %v", err)
	}
	if err := svc.Reject(a, "m1", "policy violation"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Submit(a); !errors.Is(err, ErrNotSubmittable) {
		t.Fatalf("rejected report cannot resubmit: %v", err)
	}
	// 发票释放后，b 可以提交并走审批。
	if err := svc.Submit(b); err != nil {
		t.Fatalf("invoice reused after reject: %v", err)
	}
	if err := svc.Approve(b, "m1"); err != nil {
		t.Fatal(err)
	}
	view, _ := svc.Get(b)
	if view.Status != StatusApproved {
		t.Fatalf("status = %s", view.Status)
	}
	// 已批准继续占用发票。
	c := createReady(t, svc, item("INV-SHARED", 300))
	if err := svc.Submit(c); !errors.Is(err, ErrInvoiceConflict) {
		t.Fatalf("approved report must keep invoice: %v", err)
	}
}

func TestReopenPersistsState(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	svc, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Init(1000); err != nil {
		t.Fatal(err)
	}
	approved := createReady(t, svc, item("INV-OK", 500))
	if err := svc.Submit(approved); err != nil {
		t.Fatal(err)
	}
	if err := svc.Approve(approved, "m1"); err != nil {
		t.Fatal(err)
	}
	waiting := createReady(t, svc, item("INV-BIG", 1000))
	if err := svc.Submit(waiting); err != nil {
		t.Fatal(err)
	}
	if err := svc.Approve(waiting, "m1"); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if threshold, init := reopened.Threshold(); !init || threshold != 1000 {
		t.Fatalf("threshold after reopen: %d init=%v", threshold, init)
	}
	v, err := reopened.Get(waiting)
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != StatusPendingFinance || v.CurrentApprover != "f1" {
		t.Fatalf("waiting status=%s approver=%s", v.Status, v.CurrentApprover)
	}
	if len(v.History) != 2 || v.History[0].Action != "submit" || v.History[1].Action != "approve" {
		t.Fatalf("history not persisted: %+v", v.History)
	}
	if v.TotalCents != 1000 {
		t.Fatalf("total = %d", v.TotalCents)
	}
	// 占用关系也要恢复。
	conflict := createReady(t, reopened, item("INV-OK", 1))
	if err := reopened.Submit(conflict); !errors.Is(err, ErrInvoiceConflict) {
		t.Fatalf("invoice occupancy not restored: %v", err)
	}
}

func TestListFilterAndSort(t *testing.T) {
	svc := newService(t, 10000)
	a := createReady(t, svc, item("INV-1", 100))
	b := createReady(t, svc, item("INV-2", 100))
	_ = a
	if err := svc.Submit(b); err != nil {
		t.Fatal(err)
	}
	list, err := svc.List("", StatusDraft)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != "R1" {
		t.Fatalf("draft filter: %+v", list)
	}
	all, _ := svc.List("u1", "")
	if len(all) != 2 || all[0].ID != "R1" || all[1].ID != "R2" {
		t.Fatalf("sorted list: %+v", all)
	}
	none, _ := svc.List("nobody", "")
	if len(none) != 0 {
		t.Fatalf("applicant filter: %+v", none)
	}
}
