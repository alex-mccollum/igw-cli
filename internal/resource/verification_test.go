package resource

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/execute"
	"github.com/alex-mccollum/igw-cli/internal/result"
)

func TestVerificationChecksExplainUncertainCreate(t *testing.T) {
	for _, tc := range []struct {
		name, ack, after       string
		acknowledged, readback bool
		mismatches             []string
	}{
		{"ignored field", `{"success":true,"changes":[{"type":"test/settings","collection":"core","newSignature":"after"}]}`, `{"type":"test/settings","collection":"core","signature":"after","enabled":false,"description":"private-normalized"}`, true, true, []string{"description", "enabled"}},
		{"signature drift", `{"success":true,"changes":[{"type":"test/settings","collection":"core","newSignature":"different"}]}`, `{"type":"test/settings","collection":"core","signature":"after","enabled":true,"description":"private-requested"}`, true, true, nil},
		{"unknown acknowledgement", `{}`, `{"type":"test/settings","collection":"core","signature":"after","enabled":true,"description":"private-requested"}`, false, true, nil},
		{"invalid readback", `{"success":true}`, `{}`, true, false, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			got := Apply(runFunc(func(execute.Request) result.Result {
				calls++
				switch calls {
				case 1:
					return missing()
				case 2:
					return response(tc.ack)
				case 3:
					return response(tc.after)
				}
				t.Fatal("write replayed")
				return result.Result{}
			}), Change{Action: "create", Type: "test/settings", Singleton: true, Collection: "core", Yes: true, Body: []byte(`{"enabled":true,"description":"private-requested"}`)})
			if got.OK || got.Outcome != "uncertain" || calls != 3 {
				t.Fatalf("unexpected create outcome: %+v", got)
			}
			checks := got.Data.(Evidence).Checks
			if checks == nil || checks.Acknowledged != tc.acknowledged || checks.ReadbackValid != tc.readback {
				t.Fatalf("lost verification checks: %+v", checks)
			}
			if !tc.readback {
				if checks.FieldsMatched != nil || checks.SignatureMatched != nil {
					t.Fatal("unavailable comparisons were reported as performed")
				}
			} else {
				if checks.FieldsMatched == nil || checks.SignatureMatched == nil || *checks.FieldsMatched != (len(tc.mismatches) == 0) || len(checks.MismatchedFields) != len(tc.mismatches) {
					t.Fatalf("incorrect comparisons: %+v", checks)
				}
				if len(tc.mismatches) > 0 && !reflect.DeepEqual(checks.MismatchedFields, tc.mismatches) {
					t.Fatal("mismatch names are not deterministic")
				}
				if *checks.SignatureMatched != (tc.name == "ignored field") {
					t.Fatal("signature mismatch was not distinguished")
				}
			}
			encoded, _ := json.Marshal(got)
			if strings.Contains(string(encoded), "private-") {
				t.Fatal("verification checks disclosed configuration values")
			}
		})
	}
}
