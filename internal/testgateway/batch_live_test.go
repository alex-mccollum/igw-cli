package testgateway_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/catalog"
	"github.com/alex-mccollum/igw-cli/internal/execute"
)

type inputBatchItemEvidence struct {
	ID         string           `json:"id"`
	Outcome    string           `json:"outcome"`
	HTTPStatus int              `json:"httpStatus,omitempty"`
	ErrorKind  string           `json:"errorKind,omitempty"`
	ExitCode   int              `json:"exitCode"`
	Validation string           `json:"validation,omitempty"`
	Preview    *execute.Preview `json:"preview,omitempty"`
}

type inputBatchEvidence struct {
	Items     []inputBatchItemEvidence `json:"items"`
	Succeeded int                      `json:"succeeded"`
	Failed    int                      `json:"failed"`
	NotRun    int                      `json:"notRun"`
}

type liveBatchReport struct {
	Items []struct {
		ID     string         `json:"id"`
		Result transferResult `json:"result"`
	} `json:"items"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
	NotRun    int `json:"notRun"`
}

// Retain per-item outcomes and previews, never returned ciphertext, response
// bodies, or error detail values. The live assertions inspect those in memory.
func batchEvidence(raw json.RawMessage) *inputBatchEvidence {
	var report liveBatchReport
	if json.Unmarshal(raw, &report) != nil || report.Items == nil {
		return nil
	}
	evidence := &inputBatchEvidence{Items: make([]inputBatchItemEvidence, len(report.Items)), Succeeded: report.Succeeded, Failed: report.Failed, NotRun: report.NotRun}
	for n, item := range report.Items {
		r := item.Result
		e := inputBatchItemEvidence{ID: item.ID, Outcome: r.Outcome, HTTPStatus: inputHTTPStatus(r), Validation: r.Meta.Validation}
		if r.Error != nil {
			e.ErrorKind, e.ExitCode = r.Error.Kind, r.Error.Code
		}
		if r.Outcome == "preview" {
			var preview execute.Preview
			if json.Unmarshal(r.Data, &preview) == nil {
				e.Preview = &preview
			}
		}
		evidence.Items[n] = e
	}
	return evidence
}

func TestLiveBatch(t *testing.T) {
	s := beginObservedSuite(t, "IGW_BATCH_EVIDENCE_DIR", "batch-workflows", "batch.json")
	read := map[string]any{"id": "before", "operation": "GET /data/api/v1/gateway-info"}
	write := map[string]any{"id": "encrypt", "operation": "POST /data/api/v1/encryption/encrypt", "bodyText": "igw batch public fixture + & 日本", "contentType": "text/plain"}
	after := map[string]any{"id": "after", "operation": "GET /data/api/v1/gateway-info"}
	input := func(items ...map[string]any) string {
		raw, err := json.Marshal(items)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	run := func(name, manifest string, requests int64, catalogRequests int, outcome string, code int, want []string, flags ...string) liveBatchReport {
		t.Helper()
		args := append([]string{"api", "batch", "--input", manifest}, flags...)
		got := s.run(name, nil, args...)
		check := s.last()
		if got.Outcome != outcome || got.OK != (code == 0) || check.ExitCode != code || check.OperationRequests != requests || check.CatalogRequests != catalogRequests {
			t.Fatalf("%s: wrong outcome/code/request counts: %+v", name, check.transferCheck)
		}
		var report liveBatchReport
		if want == nil {
			if check.Batch != nil {
				t.Fatal("malformed input fabricated item results")
			}
			return report
		}
		if json.Unmarshal(got.Data, &report) != nil || len(report.Items) != len(want) || check.Batch == nil {
			t.Fatal("batch lost item results")
		}
		passed, failed, notRun := 0, 0, 0
		for n, item := range report.Items {
			if item.Result.Outcome != want[n] {
				t.Fatalf("%s item %s: got %s want %s", name, item.ID, item.Result.Outcome, want[n])
			}
			if item.Result.Outcome == "not_run" {
				notRun++
				if item.Result.OK || item.Result.Error != nil || string(item.Result.Data) != "null" {
					t.Fatal("unattempted item fabricated data")
				}
				continue
			}
			if item.Result.OK {
				passed++
			} else {
				failed++
			}
			m := item.Result.Meta.Catalog
			if m == nil || m.ContractSHA256 != s.receipt.Catalog.ContractSHA256 || m.ParserVersion != catalog.ParserVersion {
				t.Fatal("batch item lost invocation catalog identity")
			}
			if item.Result.Outcome == "completed" {
				var data map[string]json.RawMessage
				if json.Unmarshal(item.Result.Data, &data) != nil || len(data) == 0 {
					t.Fatal("batch lost successful Gateway response data")
				}
			}
			if item.Result.Outcome == "accepted" && !inputJWE(item.Result.Data, false) {
				t.Fatal("batch lost accepted encryption response")
			}
			if item.Result.Outcome == "preview" && check.Batch.Items[n].Preview == nil {
				t.Fatal("batch lost prepared input identity")
			}
		}
		if report.Succeeded != passed || report.Failed != failed || report.NotRun != notRun {
			t.Fatal("batch counts differ from item outcomes")
		}
		return report
	}
	complete := input(read, write, after)
	run("malformed-late-field", input(read, map[string]any{"id": "invalid", "operation": "GET /data/api/v1/gateway-info", "unknown": true}), 0, 0, "failed", 2, nil, "--yes")
	run("duplicate-id", input(read, read), 0, 0, "failed", 2, nil, "--yes")
	run("confirmation", complete, 0, 0, "failed", 2, []string{"not_run", "not_run", "not_run"})
	for _, offline := range []bool{false, true} {
		name, flags := "preview", []string{"--dry-run"}
		if offline {
			name = "offline-preview"
			flags = append(flags, "--offline")
		}
		run(name, complete, 0, 0, "preview", 0, []string{"preview", "preview", "preview"}, flags...)
		p := s.last().Batch.Items[1].Preview
		if !p.BodyPresent || p.BodySHA256 != inputDigest([]byte(write["bodyText"].(string))) || p.Validation != catalog.ValidationSchema {
			t.Fatal("batch preview changed exact text input")
		}
	}
	run("read-only", input(read, after), 2, 0, "completed", 0, []string{"completed", "completed"})
	run("complete", complete, 3, 1, "accepted", 0, []string{"completed", "accepted", "completed"}, "--yes")
	bad := map[string]any{"id": "invalid", "operation": "POST /data/api/v1/encryption/encrypt"}
	for _, keepGoing := range []bool{false, true} {
		name, flags, requests, last := "stop-validation", []string{"--yes"}, int64(1), "not_run"
		if keepGoing {
			name, flags, requests, last = "continue-validation", append(flags, "--continue-on-error"), 2, "completed"
		}
		report := run(name, input(write, bad, after), requests, 1, "partial", 2, []string{"accepted", "failed", last}, flags...)
		if report.Items[1].Result.Error.Kind != "validation" {
			t.Fatal("invalid body did not fail as validation")
		}
	}
	missing := map[string]any{"id": "missing", "operation": "GET /data/api/v1/resources/datafile/ignition/translations/{filename}", "pathParams": map[string]string{"filename": "igw-batch-missing.bin"}, "query": map[string][]string{"collection": {"core"}}}
	for _, keepGoing := range []bool{false, true} {
		name, flags, requests, last := "stop-http", []string{}, int64(2), "not_run"
		if keepGoing {
			name, flags, requests, last = "continue-http", []string{"--continue-on-error"}, 3, "completed"
		}
		report := run(name, input(read, missing, after), requests, 0, "partial", 7, []string{"completed", "failed", last}, flags...)
		if report.Items[1].Result.Error.Kind != "http" || inputHTTPStatus(report.Items[1].Result) < 400 {
			t.Fatal("missing datafile did not exercise ordinary HTTP failure")
		}
	}
	unknown := map[string]any{"id": "unknown", "operation": "GET /__igw_missing_batch_operation__"}
	run("continue-unknown-operation", input(read, unknown, after), 2, 0, "partial", 2, []string{"completed", "failed", "completed"}, "--continue-on-error")
	s.completed = true
}

func TestBatchEvidenceOmitsResponseValues(t *testing.T) {
	raw := json.RawMessage(`{"items":[{"id":"success","result":{"ok":true,"outcome":"accepted","data":{"ciphertext":"private-returned-data"},"meta":{"httpStatus":200,"validation":"declared_schema"}}},{"id":"error","result":{"ok":false,"outcome":"failed","data":null,"error":{"kind":"http","message":"private-message","exitCode":7,"details":{"httpStatus":500,"private":"private-detail"}},"meta":{}}}],"succeeded":1,"failed":1,"notRun":0}`)
	evidence := batchEvidence(raw)
	if evidence == nil || len(evidence.Items) != 2 || evidence.Items[1].HTTPStatus != 500 || evidence.Items[1].ExitCode != 7 {
		t.Fatal("evidence lost status")
	}
	encoded, _ := json.Marshal(evidence)
	if strings.Contains(string(encoded), "private-") || strings.Contains(string(encoded), "ciphertext") {
		t.Fatal("batch evidence retained response values")
	}
}
