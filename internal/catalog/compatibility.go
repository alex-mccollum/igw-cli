package catalog

import (
	"sort"
	"strconv"
	"strings"
)

const compatibilityPolicy = "ignition-openapi/1"

// Compatibility describes the model adapter, not a claim that original vendor
// bytes satisfy the OAS schema. Receipts bind this policy to the raw SHA-256.
type Compatibility struct {
	Policy      string         `json:"policy"`
	Adjustments int            `json:"adjustments"`
	Rules       map[string]int `json:"rules"`
}

type Adjustment struct {
	Operation string `json:"operation"`
	Pointer   string `json:"pointer"`
	Rule      string `json:"rule"`
}

// normalizeIgnition handles only two reviewed structural defects captured from
// IA's 8.3 API generator. It never rewrites request constraints or invents a
// response status/schema. Original bytes and descriptions are kept separately.
func normalizeIgnition(value any) []Adjustment {
	root, _ := value.(map[string]any)
	info, _ := root["info"].(map[string]any)
	license, _ := info["license"].(map[string]any)
	if root["openapi"] != "3.1.0" || info["title"] != "Ignition HTTP API" || license["url"] != "https://inductiveautomation.com/ignition/license" {
		return nil
	}
	paths, _ := root["paths"].(map[string]any)
	var adjustments []Adjustment
	for path, rawItem := range paths {
		item, _ := rawItem.(map[string]any)
		for _, method := range []string{"get", "put", "post", "delete", "options", "head", "patch", "trace"} {
			op, ok := item[method].(map[string]any)
			if !ok {
				continue
			}
			key := strings.ToUpper(method) + " " + path
			pointer := "/paths/" + pointerEscape(path) + "/" + method
			params, _ := op["parameters"].([]any)
			for index, rawParam := range params {
				param, _ := rawParam.(map[string]any)
				if param["in"] == "path" && param["allowReserved"] == false {
					delete(param, "allowReserved")
					adjustments = append(adjustments, Adjustment{Operation: key, Pointer: pointer + "/parameters/" + strconv.Itoa(index) + "/allowReserved", Rule: "path-allow-reserved-false"})
				}
			}
			if responses, ok := op["responses"].(map[string]any); ok && len(responses) == 0 {
				responses["default"] = map[string]any{"description": "Response undocumented by the Gateway; placeholder for parser compatibility only."}
				adjustments = append(adjustments, Adjustment{Operation: key, Pointer: pointer + "/responses", Rule: "empty-responses"})
			}
		}
	}
	sort.Slice(adjustments, func(i, j int) bool { return adjustments[i].Pointer < adjustments[j].Pointer })
	return adjustments
}

func pointerEscape(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1")
}

func (c *Catalog) Compatibility() *Compatibility {
	if len(c.adjustments) == 0 {
		return nil
	}
	counts := make(map[string]int)
	for _, adjustment := range c.adjustments {
		counts[adjustment.Rule]++
	}
	return &Compatibility{Policy: compatibilityPolicy, Adjustments: len(c.adjustments), Rules: counts}
}

func (c *Catalog) Adjustments() []Adjustment { return append([]Adjustment(nil), c.adjustments...) }
