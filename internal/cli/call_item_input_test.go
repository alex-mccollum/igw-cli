package cli

import (
	"testing"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func TestBuildCallExecutionInputFromItemDefaultsAndOverrides(t *testing.T) {
	t.Parallel()

	yes := true
	retry := 3
	item := callBatchItem{
		Method:       "GET",
		Path:         "/data/api/v1/gateway-info",
		Timeout:      "5s",
		Retry:        &retry,
		RetryBackoff: "750ms",
		Yes:          &yes,
		DryRun:       true,
	}

	input, err := buildCallExecutionInputFromItem(item, callItemExecutionDefaults{
		Timeout:      8 * time.Second,
		Retry:        1,
		RetryBackoff: 250 * time.Millisecond,
		Yes:          false,
		EnableTiming: true,
	})
	if err != nil {
		t.Fatalf("buildCallExecutionInputFromItem failed: %v", err)
	}

	if input.Timeout != 5*time.Second {
		t.Fatalf("unexpected timeout %s", input.Timeout)
	}
	if input.Retry != 3 {
		t.Fatalf("unexpected retry %d", input.Retry)
	}
	if input.RetryBackoff != 750*time.Millisecond {
		t.Fatalf("unexpected retry backoff %s", input.RetryBackoff)
	}
	if !input.Yes {
		t.Fatalf("expected yes=true override")
	}
	if !input.DryRun {
		t.Fatalf("expected dryRun=true passthrough")
	}
}

func TestBuildCallExecutionInputFromItemRejectsInvalidDurations(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		item callBatchItem
	}{
		{
			name: "timeout",
			item: callBatchItem{
				Path:    "/x",
				Timeout: "bad",
			},
		},
		{
			name: "retryBackoff",
			item: callBatchItem{
				Path:         "/x",
				RetryBackoff: "bad",
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := buildCallExecutionInputFromItem(tc.item, callItemExecutionDefaults{
				Timeout:      time.Second,
				RetryBackoff: 250 * time.Millisecond,
			})
			if err == nil {
				t.Fatalf("expected usage error")
			}
			if _, ok := err.(*igwerr.UsageError); !ok {
				t.Fatalf("expected usage error type, got %T", err)
			}
		})
	}
}
