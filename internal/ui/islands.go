package ui

import (
	"encoding/json"
	"log"
)

// islandPropsJSON serializes v to a JSON string for use as the data-props
// attribute value. It returns the raw JSON string — templ's attribute
// interpolation (EscapeString) will HTML-encode it when writing the attribute,
// so we must NOT pre-escape here to avoid double-encoding.
//
// On marshal failure it logs the error and returns "{}" so the island receives
// empty props rather than a broken page.
func islandPropsJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		log.Printf("ui/islands: failed to marshal island props: %v", err)
		return "{}"
	}
	return string(b)
}
