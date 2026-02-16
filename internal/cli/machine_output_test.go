package cli

import (
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func TestArgsWantJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "explicit json flag", args: []string{"--json"}, want: true},
		{name: "json true value", args: []string{"--json=true"}, want: true},
		{name: "json false value", args: []string{"--json=false"}, want: false},
		{name: "json uppercase bool", args: []string{"--json=TRUE"}, want: true},
		{name: "missing json", args: []string{"--path", "/x"}, want: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := argsWantJSON(tc.args)
			if got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestJSONErrorPayloadUsage(t *testing.T) {
	t.Parallel()

	payload := jsonErrorPayload(&igwerr.UsageError{Msg: "bad input"})
	if payload["ok"] != false {
		t.Fatalf("expected ok=false, got %v", payload["ok"])
	}
	if int(payload["code"].(int)) != 2 {
		t.Fatalf("expected code=2, got %v", payload["code"])
	}
	if payload["error"] != "bad input" {
		t.Fatalf("unexpected error text: %v", payload["error"])
	}
	if _, exists := payload["details"]; exists {
		t.Fatalf("expected no details for usage error")
	}
}

func TestJSONErrorPayloadStatusDetails(t *testing.T) {
	t.Parallel()

	payload := jsonErrorPayload(&igwerr.StatusError{
		StatusCode: 403,
		Hint:       "forbidden",
	})
	details, ok := payload["details"].(map[string]any)
	if !ok {
		t.Fatalf("expected details map")
	}
	if int(details["status"].(int)) != 403 {
		t.Fatalf("expected status=403, got %v", details["status"])
	}
	if details["hint"] != "forbidden" {
		t.Fatalf("expected hint, got %v", details["hint"])
	}
}

func TestJSONErrorPayloadTransportTimeoutDetails(t *testing.T) {
	t.Parallel()

	payload := jsonErrorPayload(&igwerr.TransportError{Timeout: true})
	details, ok := payload["details"].(map[string]any)
	if !ok {
		t.Fatalf("expected details map")
	}
	if details["timeout"] != true {
		t.Fatalf("expected timeout=true, got %v", details["timeout"])
	}
}
