package main

import (
	"bytes"
	"context"
	"testing"
)

func TestQualifyHelpAndUsageAreOffline(t *testing.T) {
	for _, tc := range []struct {
		args []string
		code int
	}{
		{[]string{"--help"}, 0}, {nil, 2}, {[]string{"--unknown"}, 2},
		{[]string{"--out", "unused", "--timeout", "0"}, 2},
		{[]string{"--out", "unused", "extra"}, 2},
	} {
		var out, diagnostic bytes.Buffer
		if code := runQualify(context.Background(), tc.args, &out, &diagnostic); code != tc.code || out.Len() != 0 || diagnostic.Len() == 0 {
			t.Fatalf("arguments %q: code=%d stdout=%s stderr=%s", tc.args, code, &out, &diagnostic)
		}
	}
}
