package cli

import "testing"

func TestDecodeCallStatsPayloadVersionContract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     any
		wantErr string
	}{
		{
			name: "typed stats supported version",
			raw: callStats{
				Version:   callStatsSchemaVersion,
				TimingMs:  12,
				BodyBytes: 34,
			},
		},
		{
			name: "map stats supported version",
			raw: map[string]any{
				"version":   float64(callStatsSchemaVersion),
				"timingMs":  float64(12),
				"bodyBytes": float64(34),
			},
		},
		{
			name: "typed stats unsupported version",
			raw: callStats{
				Version:   2,
				TimingMs:  12,
				BodyBytes: 34,
			},
			wantErr: "unsupported stats.version 2",
		},
		{
			name: "map stats unsupported version",
			raw: map[string]any{
				"version":   float64(2),
				"timingMs":  float64(12),
				"bodyBytes": float64(34),
			},
			wantErr: "unsupported stats.version 2",
		},
		{
			name:    "missing payload",
			raw:     nil,
			wantErr: "missing stats payload",
		},
		{
			name:    "unsupported payload type",
			raw:     "nope",
			wantErr: "unsupported stats payload type string",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			stats, err := decodeCallStatsPayload(tc.raw)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("decode failed: %v", err)
				}
				if stats.Version != callStatsSchemaVersion {
					t.Fatalf("expected version %d, got %d", callStatsSchemaVersion, stats.Version)
				}
				return
			}

			if err == nil {
				t.Fatalf("expected error containing %q", tc.wantErr)
			}
			if got := err.Error(); got != tc.wantErr {
				t.Fatalf("unexpected error: got=%q want=%q", got, tc.wantErr)
			}
		})
	}
}
