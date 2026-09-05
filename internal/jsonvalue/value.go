package jsonvalue

import (
	"bytes"
	"encoding/json"
	"io"
	"math/big"
	"reflect"
	"strings"

	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

// Equivalent compares values without rounding numbers. Subset permits extra
// object properties, including defaults supplied by the Gateway.
func Equivalent(a, b []byte, subset bool) bool {
	if len(a) == 0 || len(b) == 0 {
		return len(a) == 0 && len(b) == 0
	}
	decode := func(raw []byte) (any, error) {
		var value any
		d := json.NewDecoder(bytes.NewReader(raw))
		d.UseNumber()
		err := d.Decode(&value)
		return value, err
	}
	left, errA := decode(a)
	right, errB := decode(b)
	return errA == nil && errB == nil && sameValue(left, right, subset)
}

// Canonical rejects ambiguous JSON and normalizes key order and bounded number
// spellings. It does not reorder arrays or strip application fields.
func Canonical(raw []byte) ([]byte, error) {
	if err := Validate(raw); err != nil {
		return nil, err
	}
	var value any
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if err := d.Decode(&value); err != nil {
		return nil, err
	}
	var normalize func(any) any
	normalize = func(value any) any {
		switch v := value.(type) {
		case map[string]any:
			for key, child := range v {
				v[key] = normalize(child)
			}
		case []any:
			for i, child := range v {
				v[i] = normalize(child)
			}
		case json.Number:
			if len(v) <= 128 {
				return json.Number(numberKey(string(v)))
			}
		}
		return value
	}
	return json.Marshal(normalize(value))
}

// Reject duplicate keys even inside config. Otherwise body normalization could
// silently select a different value than the user reviewed.
func Validate(raw []byte) error {
	bad := &igwerr.UsageError{Msg: "body must be valid JSON without duplicate keys or excessive nesting"}
	if len(raw) > 32<<20 {
		return &igwerr.UsageError{Msg: "resource body exceeds 32 MiB"}
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var value func(int) error
	value = func(depth int) error {
		if depth > 256 {
			return bad
		}
		token, err := d.Token()
		if err != nil {
			return bad
		}
		delim, composite := token.(json.Delim)
		if !composite {
			return nil
		}
		if delim != '{' && delim != '[' {
			return bad
		}
		seen := map[string]bool{}
		for d.More() {
			if delim == '{' {
				token, err := d.Token()
				key, ok := token.(string)
				if err != nil || !ok || seen[key] {
					return bad
				}
				seen[key] = true
			}
			if err := value(depth + 1); err != nil {
				return err
			}
		}
		end, err := d.Token()
		if err != nil || delim == '{' && end != json.Delim('}') || delim == '[' && end != json.Delim(']') {
			return bad
		}
		return nil
	}
	if err := value(0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return bad
	}
	return nil
}

func sameValue(a, b any, subset bool) bool {
	switch a := a.(type) {
	case map[string]any:
		other, ok := b.(map[string]any)
		if !ok || !subset && len(a) != len(other) {
			return false
		}
		for key, value := range a {
			got, exists := other[key]
			if !exists || !sameValue(value, got, subset) {
				return false
			}
		}
		return true
	case []any:
		other, ok := b.([]any)
		if !ok || len(a) != len(other) {
			return false
		}
		for n, v := range a {
			if !sameValue(v, other[n], subset) {
				return false
			}
		}
		return true
	case json.Number:
		other, ok := b.(json.Number)
		if !ok {
			return false
		}
		if a == other {
			return true
		}
		if len(a) > 128 || len(other) > 128 {
			return false
		}
		return numberKey(string(a)) == numberKey(string(other))
	default:
		return reflect.DeepEqual(a, b)
	}
}

// Normalize decimal digits and a symbolic exponent, never expanding a large
// exponent or rounding through floating point. Inputs already parsed as JSON.
func numberKey(text string) string {
	mantissa, exponent, found := strings.Cut(strings.ToLower(text), "e")
	var power big.Int
	if found {
		power.SetString(exponent, 10)
	}
	negative := strings.HasPrefix(mantissa, "-")
	mantissa = strings.TrimPrefix(mantissa, "-")
	whole, fraction, _ := strings.Cut(mantissa, ".")
	digits := strings.TrimLeft(whole+fraction, "0")
	if digits == "" {
		return "0"
	}
	trimmed := strings.TrimRight(digits, "0")
	power.Add(&power, big.NewInt(int64(len(digits)-len(trimmed)-len(fraction))))
	if negative {
		trimmed = "-" + trimmed
	}
	return trimmed + "e" + power.String()
}
