package doctor

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// prettyJSON renders an arbitrary value (e.g. UpgradeState.Findings, typed as
// interface{}) as indented JSON for display in a <pre> block. Falls back to a
// plain %v representation if the value cannot be marshalled.
func prettyJSON(v interface{}) string {
	if v == nil {
		return ""
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

// truncate returns at most n runes from s, appending an ellipsis when truncated.
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}

// projectSlug sanitizes a project key for safe use inside HTML id/class
// attributes and CSS/HTMX selectors. Any character outside [A-Za-z0-9_-] is
// replaced with '-', so a project key like "e:/Foo Bar" never produces an
// invalid id or a selector that fails to match.
func projectSlug(project string) string {
	var b strings.Builder
	b.Grow(len(project))
	for _, r := range project {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return b.String()
}

// syncProjectURL builds a path-escaped /doctor/sync URL for a project, with an
// optional trailing action segment ("enroll", "unenroll", "sync"). Escaping the
// project segment keeps URLs valid when the key contains spaces or reserved
// characters. Pass action="" for the project detail URL.
func syncProjectURL(project, action string) string {
	p := "/doctor/sync/" + url.PathEscape(project)
	if action == "" {
		return p
	}
	return p + "/" + action
}

// orphanActionURL builds a path-escaped /doctor/orphans URL for an entity,
// with an optional trailing action segment ("project" for assign). Pass
// action="" for the delete URL.
func orphanActionURL(entity, id, action string) string {
	p := "/doctor/orphans/" + entity + "/" + url.PathEscape(id)
	if action == "" {
		return p
	}
	return p + "/" + action
}
