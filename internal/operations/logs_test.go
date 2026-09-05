package operations

import (
	"testing"
	"time"
)

func TestLogTimeWindowAndQuery(t *testing.T) {
	now := time.Date(2026, 9, 5, 15, 0, 0, 123000000, time.UTC)
	since, err := LogTime("1h", now, true)
	if err != nil {
		t.Fatal(err)
	}
	until, err := LogTime("2026-09-05T08:00:00.123-07:00", now, false)
	if err != nil {
		t.Fatal(err)
	}
	q, err := (LogQuery{Limit: 25, Offset: 10, MinLevel: "warn", Logger: "A&B", Since: since, Until: until}).Values()
	if err != nil || q.Get("minLevel") != "WARN" || q.Get("logger") != "A&B" || q.Get("limit") != "25" || q.Get("offset") != "10" || !until.Equal(now) || now.Sub(*since) != time.Hour {
		t.Fatalf("incorrect filter conversion: %v %v", q, err)
	}
	if q.Get("endTime") != "1788620400123" || q.Get("startTime") != "1788616800123" {
		t.Fatalf("epoch millisecond conversion changed: %v", q)
	}
}

func TestLogFiltersRejectInvalidInput(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-time.Second)
	for _, q := range []LogQuery{{Limit: 0}, {Limit: 1001}, {Limit: 10, Offset: -1}, {Limit: 10, MinLevel: "fatal-ish"}, {Limit: 10, Since: &now, Until: &earlier}} {
		if _, err := q.Values(); err == nil {
			t.Fatal("invalid log query accepted")
		}
	}
	for _, value := range []string{"-1h", "0s", "tomorrow", "9223372036854775807h"} {
		if _, err := LogTime(value, now, true); err == nil {
			t.Fatal("invalid relative time accepted")
		}
	}
	if _, err := LogTime("1h", now, false); err == nil {
		t.Fatal("relative end time accepted")
	}
}
