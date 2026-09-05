package main

import (
	"bytes"
	"context"
	"testing"
)

func TestResolveHelpAndUsageAreOffline(t *testing.T) {
	for _, tc := range []struct {
		args []string
		code int
	}{
		{[]string{"--help"}, 0}, {nil, 2}, {[]string{"--unknown"}, 2},
		{[]string{"--out", "unused", "--timeout", "0"}, 2},
		{[]string{"--out", "unused", "--timeout", "61s"}, 2},
		{[]string{"--out", "unused", "extra"}, 2},
	} {
		var stdout, stderr bytes.Buffer
		if got := runResolve(context.Background(), tc.args, &stdout, &stderr); got != tc.code || stdout.Len() != 0 || stderr.Len() == 0 {
			t.Fatalf("args %q: code=%d out=%s err=%s", tc.args, got, &stdout, &stderr)
		}
	}
}
