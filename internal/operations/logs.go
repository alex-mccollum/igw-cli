// Package operations implements typed Gateway operational workflows.
package operations

import (
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/result"
)

type LogQuery struct {
	Limit, Offset            int
	MinLevel, Logger, Search string
	Since, Until             *time.Time
}

func (q LogQuery) Values() (url.Values, error) {
	if q.Limit < 1 || q.Limit > 1000 || q.Offset < 0 {
		return nil, result.Usage("limit must be 1..1000 and offset must be nonnegative")
	}
	values := url.Values{"limit": {strconv.Itoa(q.Limit)}, "offset": {strconv.Itoa(q.Offset)}}
	if q.MinLevel != "" {
		level := strings.ToUpper(q.MinLevel)
		switch level {
		case "TRACE", "DEBUG", "INFO", "WARN", "ERROR", "FATAL", "OFF":
		default:
			return nil, result.Usage("minimum log level must be TRACE, DEBUG, INFO, WARN, ERROR, FATAL, or OFF")
		}
		values.Set("minLevel", level)
	}
	if q.Logger != "" {
		values.Set("logger", q.Logger)
	}
	if q.Search != "" {
		values.Set("search", q.Search)
	}
	if q.Since != nil {
		values.Set("startTime", strconv.FormatInt(q.Since.UnixMilli(), 10))
	}
	if q.Until != nil {
		values.Set("endTime", strconv.FormatInt(q.Until.UnixMilli(), 10))
	}
	if q.Since != nil && q.Until != nil && q.Since.After(*q.Until) {
		return nil, result.Usage("--since must not be later than --until")
	}
	return values, nil
}

// LogTime accepts an absolute RFC3339 instant or, for --since, a positive
// duration measured backwards from one invocation clock reading.
func LogTime(value string, now time.Time, relative bool) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return &parsed, nil
	}
	if relative {
		if duration, err := time.ParseDuration(value); err == nil && duration > 0 {
			parsed := now.Add(-duration)
			return &parsed, nil
		}
	}
	return nil, result.Usage("log times must be RFC3339 timestamps; --since also accepts a positive duration such as 1h")
}
