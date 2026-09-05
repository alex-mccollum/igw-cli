package gateway

import (
	"net/http"
	"sort"
	"strings"

	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

// ParseHeader accepts a command-line field line without echoing sensitive input
// in errors. Only HTTP optional whitespace (SP and HTAB) is stripped.
func ParseHeader(pair string) (string, string, error) {
	name, value, ok := strings.Cut(pair, ":")
	if !ok {
		return "", "", &igwerr.UsageError{Msg: "header requires name:value"}
	}
	return normalizeHeaderField(strings.Trim(name, " \t"), value)
}

// NormalizeHeaders returns an independent map matching the transport's field
// values. Merge case variants in sorted key order; values within a field keep
// their order. An empty value slice emits no field, unlike a slice containing "".
func NormalizeHeaders(input http.Header) (http.Header, error) {
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	output := make(http.Header, len(keys))
	for _, key := range keys {
		name, _, err := normalizeHeaderField(key, "")
		if err != nil {
			return nil, err
		}
		for _, value := range input[key] {
			_, value, err = normalizeHeaderField(name, value)
			if err != nil {
				return nil, err
			}
			output[name] = append(output[name], value)
		}
	}
	return output, nil
}

func normalizeHeaderField(name, value string) (string, string, error) {
	if name == "" {
		return "", "", &igwerr.UsageError{Msg: "invalid header name"}
	}
	for i := range len(name) {
		b := name[i]
		if !(b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", rune(b))) {
			return "", "", &igwerr.UsageError{Msg: "invalid header name"}
		}
	}
	for i := range len(value) {
		b := value[i]
		if b < 0x20 && b != '\t' || b == 0x7f {
			return "", "", &igwerr.UsageError{Msg: "invalid header value"}
		}
	}
	return http.CanonicalHeaderKey(name), strings.Trim(value, " \t"), nil
}
