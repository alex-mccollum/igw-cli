package execute

import (
	"net/url"
	"unicode/utf8"

	"github.com/alex-mccollum/igw-cli/internal/catalog"
	"github.com/alex-mccollum/igw-cli/internal/result"
)

// encodeForm takes literal field values. It never guesses JSON types, splits
// delimiters, or decodes percent escapes. Values.Encode sorts names and keeps
// each name's supplied value order. Limit escaped size before allocating it.
func encodeForm(fields url.Values) ([]byte, error) {
	size, pairs := 0, 0
	for name, values := range fields {
		if name == "" || len(values) == 0 || !utf8.ValidString(name) {
			return nil, result.Usage("URL-encoded fields require a nonempty UTF-8 name and at least one value")
		}
		for _, value := range values {
			pairs++
			if pairs > catalog.MaxFormFields || !utf8.ValidString(value) {
				return nil, result.Usage("URL-encoded input exceeds the field limit or contains invalid UTF-8")
			}
			if pairs > 1 {
				size++ // &
			}
			size++ // =
			for _, text := range []string{name, value} {
				for j := 0; j < len(text); j++ {
					b := text[j]
					size++
					if !((b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '-' || b == '_' || b == '.' || b == '~' || b == ' ') {
						size += 2
					}
					if size > catalog.MaxJSONBodyBytes {
						return nil, result.Usage("URL-encoded input exceeds the 32 MiB encoded body limit")
					}
				}
			}
		}
	}
	return []byte(fields.Encode()), nil
}
