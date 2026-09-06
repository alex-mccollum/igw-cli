package catalog

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v4"
)

func TestModelDocumentPreservesParserNodes(t *testing.T) {
	// Compare actual parser node types/values, independently of JSON's float64
	// conversion. Locations and the root's presentation style may differ.
	for _, version := range []string{"3.0.1", "3.1.0"} {
		raw := `{"openapi":"` + version + `","info":{"title":"test","version":"1"},"paths":{},"components":{"schemas":{"Item":{"type":"object","properties":{"id":{"type":"integer","minimum":9007199254740993},"child":{"$ref":"#/components/schemas/Item"}}}}},"x-values":[null,true,false,[],{},0,-0,1.5,1e300,-1e-300,9007199254740993,18446744073709551615,"001","yes","a\n\r\t\f\b\u0000b","தமிழ் 🎉","a\u0085b\u2028c\u2029d","quote\"/\\"],"x-delimiters: # [ ]":{ "? key": "---\n...\n!tag &anchor *alias"}}`
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.UseNumber()
		value, err := decodeUniqueJSON(decoder, 0)
		if err != nil {
			t.Fatal(err)
		}
		baseline, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		prepared, err := modelDocument(value)
		if err != nil {
			t.Fatal(err)
		}
		before, err := newParserDocument(baseline)
		if err != nil {
			t.Fatal(err)
		}
		defer before.Release()
		after, err := newParserDocument(prepared)
		if err != nil {
			t.Fatal(err)
		}
		defer after.Release()
		type nodeValue struct {
			kind       yaml.Kind
			tag, value string
			children   int
		}
		var nodes func(*yaml.Node) []nodeValue
		nodes = func(node *yaml.Node) []nodeValue {
			out := []nodeValue{{node.Kind, node.Tag, node.Value, len(node.Content)}}
			for _, child := range node.Content {
				out = append(out, nodes(child)...)
			}
			return out
		}
		if !slices.Equal(nodes(before.GetSpecInfo().RootNode), nodes(after.GetSpecInfo().RootNode)) {
			t.Fatal("private representation changed parser node types or values")
		}
	}
}
