package gateway

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func statusHint(statusCode int) string {
	switch statusCode {
	case http.StatusUnauthorized:
		return "token missing or invalid"
	case http.StatusForbidden:
		return "token authenticated but lacks permissions or requires secure connections"
	default:
		return ""
	}
}

func shouldRetryStatus(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests || statusCode >= http.StatusInternalServerError
}

func retryDelayForResponse(statusCode int, headers http.Header, fallback time.Duration, now time.Time) time.Duration {
	if statusCode != http.StatusTooManyRequests || headers == nil {
		return fallback
	}

	raw := strings.TrimSpace(headers.Get("Retry-After"))
	if raw == "" {
		return fallback
	}

	if secs, err := time.ParseDuration(raw + "s"); err == nil {
		if secs < 0 {
			return fallback
		}
		return secs
	}

	parsedAt, err := http.ParseTime(raw)
	if err != nil {
		return fallback
	}
	wait := parsedAt.Sub(now)
	if wait < 0 {
		return 0
	}
	return wait
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return igwerr.NewTransportError(ctx.Err())
	case <-timer.C:
		return nil
	}
}
