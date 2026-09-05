package nextcli

import (
	"context"
	"testing"
	"time"
)

func TestLocalInspectionCancellationKeepsTransportExitCode(t *testing.T) {
	for _, expired := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		if expired {
			cancel()
			ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		}
		cancel()
		app, out, _ := testApp(t, nil)
		err := app.Run(ctx, []string{"project", "inspect", "unused.zip", "--json"})
		got := decodeResult(t, out)
		kind := "canceled"
		if expired {
			kind = "timeout"
		}
		if err == nil || got.OK || got.Error.Code != 7 || got.Error.Kind != kind {
			t.Fatalf("cancellation classified as invalid input: %+v", got)
		}
	}
}
