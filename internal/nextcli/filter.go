package nextcli

import (
	"net/url"
	"strings"

	"github.com/alex-mccollum/igw-cli/internal/result"
)

// Parse only the CLI spelling here. The selected Gateway's filter schema owns
// allowed fields, operators, values, and constraints in the shared executor.
func addFilters(query url.Values, filters []string) error {
	for _, input := range filters {
		key, value, ok := strings.Cut(input, "=")
		field, operator, bracket := strings.Cut(key, "[")
		if !ok || !bracket || field == "" || len(operator) < 2 || !strings.HasSuffix(operator, "]") || strings.Count(key, "[") != 1 || strings.Count(key, "]") != 1 {
			return result.Usage("filter requires field[operator]=value; repeat --filter for different keys")
		}
		if _, exists := query[key]; exists {
			return result.Usage("filter keys cannot be repeated or replace another query option")
		}
		query.Set(key, value)
	}
	return nil
}
