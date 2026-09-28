package protocol

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"example.com/travel-expense-approval/internal/domain"
)

func newSvc(t *testing.T, threshold int64) *domain.Service {
	t.Helper()
	svc, err := domain.Open(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Init(threshold); err != nil {
		t.Fatal(err)
	}
	return svc
}

func run(t *testing.T, svc *domain.Service, raw string) Response {
	t.Helper()
	resp := ProcessLine(svc, []byte(raw))
	return resp
}

func TestEndToEndFlowAndErrorCodes(t *testing.T) {
	svc := newSvc(t, 1000)

	resp := run(t, svc, `{"action":"create","applicant_id":"a","manager_id":"m","finance_id":"a"}`)
	if resp.OK || resp.Error != "people_must_be_distinct" {
		t.Fatalf("distinct people: %+v", resp)
	}

	resp = run(t, svc, `{"action":"create","applicant_id":"a","manager_id":"m","finance_id":"f"}`)
	if !resp.OK || resp.ID != "R1" {
		t.Fatalf("create: %+v", resp)
	}

	resp = run(t, svc, `{"action":"replace_items","id":"R1","items":[{"category":"餐饮","invoice_no":"","desc":"lunch","amount_cents":1}]}`)
	if resp.OK || resp.Error != "empty_invoice_no" {
		t.Fatalf("bad item: %+v", resp)
	}

	itemJSON := `{"category":"交通","invoice_no":"INV-1","desc":"taxi","amount_cents":1000}`
	resp = run(t, svc, `{"action":"replace_items","id":"R1","items":[`+itemJSON+`]}`)
	if !resp.OK {
		t.Fatalf("replace: %+v", resp)
	}

	resp = run(t, svc, `{"action":"approve","id":"R1","actor_id":"f"}`)
	if resp.OK || resp.Error != "wrong_stage_or_actor" {
		t.Fatalf("approve draft: %+v", resp)
	}

	resp = run(t, svc, `{"action":"submit","id":"R1"}`)
	if !resp.OK {
		t.Fatalf("submit: %+v", resp)
	}

	resp = run(t, svc, `{"action":"approve","id":"R1","actor_id":"m"}`)
	if !resp.OK {
		t.Fatalf("manager approve: %+v", resp)
	}

	resp = run(t, svc, `{"action":"get","id":"R1"}`)
	if !resp.OK || resp.Report.Status != "pending_finance" || resp.Report.CurrentApprover != "f" {
		t.Fatalf("get: %+v", resp)
	}
	if resp.Report.TotalCents != 1000 || len(resp.Report.History) != 2 {
		t.Fatalf("total/history: %+v", resp.Report)
	}

	resp = run(t, svc, `{"action":"approve","id":"R1","actor_id":"f"}`)
	if !resp.OK {
		t.Fatalf("finance approve: %+v", resp)
	}

	resp = run(t, svc, `{"action":"get","id":"R1"}`)
	if resp.Report.Status != "approved" || resp.Report.CurrentApprover != "" {
		t.Fatalf("approved view: %+v", resp.Report)
	}
}

func TestInvalidJSONAndUnknownAction(t *testing.T) {
	svc := newSvc(t, 1000)
	if resp := ProcessLine(svc, []byte(`{not json`)); resp.OK || resp.Error != "invalid_json" {
		t.Fatalf("invalid json: %+v", resp)
	}
	if resp := ProcessLine(svc, []byte(`{"action":"nope"}`)); resp.OK || resp.Error != "unknown_action" {
		t.Fatalf("unknown action: %+v", resp)
	}
}

func TestRejectThenListAndReuseViaJSON(t *testing.T) {
	svc := newSvc(t, 10000)
	run(t, svc, `{"action":"create","applicant_id":"a","manager_id":"m","finance_id":"f"}`)
	run(t, svc, `{"action":"replace_items","id":"R1","items":[{"category":"住宿","invoice_no":"INV-X","desc":"hotel","amount_cents":2000}]}`)
	run(t, svc, `{"action":"submit","id":"R1"}`)

	run(t, svc, `{"action":"create","applicant_id":"a","manager_id":"m","finance_id":"f"}`)
	resp := run(t, svc, `{"action":"replace_items","id":"R2","items":[{"category":"住宿","invoice_no":"INV-X","desc":"hotel2","amount_cents":50}]}`)
	_ = resp
	run(t, svc, `{"action":"submit","id":"R2"}`)

	resp = run(t, svc, `{"action":"reject","id":"R1","actor_id":"m","reason":"duplicate trip"}`)
	if !resp.OK {
		t.Fatalf("reject: %+v", resp)
	}
	resp = run(t, svc, `{"action":"submit","id":"R2"}`)
	if !resp.OK {
		t.Fatalf("reuse after reject: %+v", resp)
	}

	listResp := run(t, svc, `{"action":"list","applicant_id":"a","status":"rejected"}`)
	if !listResp.OK || len(listResp.Reports) != 1 || listResp.Reports[0].ID != "R1" {
		t.Fatalf("list rejected: %+v", listResp)
	}

	data, _ := json.Marshal(listResp)
	_ = data
}
