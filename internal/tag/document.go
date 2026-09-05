package tag

import (
	"encoding/json"
	"strings"

	"github.com/alex-mccollum/igw-cli/internal/jsonvalue"
	"github.com/alex-mccollum/igw-cli/internal/result"
)

const MaxJSONBytes int64 = 32 << 20

type Node struct {
	Name     string
	Fields   map[string]json.RawMessage
	Children []Node
}
type Document struct {
	Roots []Node
	Count int
}

// Decode recognizes a named exported tag/folder or an unnamed container with
// a tags array. It validates structural identity; the Gateway owns tag-property
// semantics, which are absent from the OpenAPI body contract.
func Decode(raw []byte) (Document, error) {
	if err := jsonvalue.Validate(raw); err != nil {
		return Document{}, err
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil || object == nil {
		return Document{}, result.Usage("tag JSON must be an object")
	}
	document := Document{}
	var parse func(map[string]json.RawMessage, int) (Node, error)
	parse = func(fields map[string]json.RawMessage, depth int) (Node, error) {
		var name string
		if depth > 128 || document.Count >= 100000 || json.Unmarshal(fields["name"], &name) != nil || name == "" || strings.ContainsAny(name, "/\\\x00\r\n") {
			return Node{}, result.Usage("tag JSON has invalid names, excessive nesting, or more than 100000 tags")
		}
		document.Count++
		node := Node{Name: name, Fields: fields}
		if children, ok := fields["tags"]; ok {
			var list []map[string]json.RawMessage
			if json.Unmarshal(children, &list) != nil || string(children) == "null" {
				return Node{}, result.Usage("tag children must be an array")
			}
			seen := map[string]bool{}
			for _, child := range list {
				n, err := parse(child, depth+1)
				if err != nil {
					return Node{}, err
				}
				if seen[n.Name] {
					return Node{}, result.Usage("tag names must be unique within each folder")
				}
				seen[n.Name] = true
				node.Children = append(node.Children, n)
			}
		}
		return node, nil
	}
	var name string
	if named, ok := object["name"]; ok && json.Unmarshal(named, &name) != nil {
		return Document{}, result.Usage("tag name must be a string")
	}
	if name != "" {
		node, err := parse(object, 0)
		if err != nil {
			return Document{}, err
		}
		document.Roots = []Node{node}
		return document, nil
	}
	var roots []map[string]json.RawMessage
	if json.Unmarshal(object["tags"], &roots) != nil || string(object["tags"]) == "null" {
		return Document{}, result.Usage("tag JSON must contain a tags array or a named tag")
	}
	seen := map[string]bool{}
	for _, root := range roots {
		node, err := parse(root, 0)
		if err != nil {
			return Document{}, err
		}
		if seen[node.Name] {
			return Document{}, result.Usage("root tag names must be unique")
		}
		seen[node.Name] = true
		document.Roots = append(document.Roots, node)
	}
	return document, nil
}

// Matches compares supplied properties and named children, allowing exported
// defaults and inherited properties. It does not claim removal of unspecified
// children or stable runtime values after this observation.
func Matches(expected, observed Node) bool {
	if expected.Name != observed.Name {
		return false
	}
	for key, want := range expected.Fields {
		if key != "tags" && !jsonvalue.Equivalent(want, observed.Fields[key], true) {
			return false
		}
	}
	children := map[string]Node{}
	for _, child := range observed.Children {
		children[child.Name] = child
	}
	for _, child := range expected.Children {
		got, ok := children[child.Name]
		if !ok || !Matches(child, got) {
			return false
		}
	}
	return true
}
