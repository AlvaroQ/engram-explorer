package services

import (
	"fmt"
	"sort"
	"strings"

	"github.com/AlvaroQ/engram-explorer/internal/sqlite"
)

// Possibly fragmented projects (Maintenance → "Possibly fragmented projects").
//
// Engram derives the project name from the agent's working directory, so the
// same project can end up under several names. SameFragment relates only the
// variants that are almost certainly the same project:
//
//   - letter case ("Foo" / "foo"),
//   - surrounding whitespace (" foo "),
//   - trailing path separators ("foo/", `foo\`),
//   - a path used as the name vs its last folder ("/Users/x/repo/foo" / "foo",
//     `C:\code\foo` / "foo").
//
// There is deliberately NO generic prefix or suffix matching: "api" and
// "API-docs", or "engram" and "engram-explorer", are usually different
// projects, and a false suggestion invites a merge that cannot be undone. The
// only "prefix" relation recognised is the path one above, where one name is
// the last folder of the other. Two different paths that only share their
// last folder ("/work/lib", "/home/lib") are NOT related: they are often two
// repositories. Results are suggestions the user confirms, never merges.

// FragmentedProjectName is one project name of a group.
type FragmentedProjectName struct {
	Name     string `json:"name"`
	Memories int    `json:"memories"` // live observations
	Sessions int    `json:"sessions"`
	Enrolled bool   `json:"enrolled"`
	// Combinable is true when this name can be merged into the group's target:
	// it is not the target and neither side is enrolled for cloud sync
	// (services.RenameProject refuses enrolled projects).
	Combinable bool `json:"combinable"`
}

// FragmentedGroup is a set of names that all relate to its Target
// (SameFragment).
type FragmentedGroup struct {
	Key string `json:"key"`
	// Target is the suggested surviving name: a plain name before a path, then
	// most memories, most sessions, the name equal to the key, the shortest.
	Target         string                  `json:"target"`
	TargetEnrolled bool                    `json:"targetEnrolled"`
	Names          []FragmentedProjectName `json:"names"`
}

// FragmentKey returns the folded last folder of a project name (lower case,
// no surrounding whitespace or trailing separators), or "" when nothing is
// left. It buckets candidates; SameFragment decides whether two names relate.
func FragmentKey(name string) string {
	s := foldName(name)
	if i := strings.LastIndexAny(s, `/\`); i >= 0 {
		s = strings.TrimSpace(s[i+1:])
	}
	return s
}

// foldName lower-cases name and drops surrounding whitespace and trailing
// path separators.
func foldName(name string) string {
	s := strings.TrimRight(strings.TrimSpace(name), `/\`)
	return strings.ToLower(strings.TrimSpace(s))
}

// isPathName reports whether a folded name still contains a path separator.
func isPathName(folded string) bool { return strings.ContainsAny(folded, `/\`) }

// SameFragment reports whether a and b are probably the same project: equal
// after folding case, whitespace and trailing separators, or one is a plain
// name equal to the last folder of the other.
func SameFragment(a, b string) bool {
	fa, fb := foldName(a), foldName(b)
	if FragmentKey(a) == "" || FragmentKey(b) == "" {
		return false
	}
	if fa == fb {
		return true
	}
	return (!isPathName(fa) && fa == FragmentKey(b)) || (!isPathName(fb) && fb == FragmentKey(a))
}

// FindFragmentedProjects returns the groups of two or more related project
// names (see splitFragments), ordered by key. Projects are the names used by live
// observations, sessions or prompts (the same set ProjectExistsDB checks).
func FindFragmentedProjects(db sqlite.Querier) ([]FragmentedGroup, error) {
	names, err := queryRows(db, `
		SELECT name, SUM(m), SUM(s) FROM (
		  SELECT project AS name, 1 AS m, 0 AS s FROM observations WHERE deleted_at IS NULL AND project IS NOT NULL
		  UNION ALL SELECT project, 0, 1 FROM sessions WHERE project IS NOT NULL
		  UNION ALL SELECT project, 0, 0 FROM user_prompts WHERE project IS NOT NULL
		) GROUP BY name`,
		nil,
		func(sc scanner) (FragmentedProjectName, error) {
			var n FragmentedProjectName
			err := sc.Scan(&n.Name, &n.Memories, &n.Sessions)
			return n, err
		},
	)
	if err != nil {
		return nil, fmt.Errorf("list project names: %w", err)
	}
	enrolled, err := queryRows(db, `SELECT project FROM sync_enrolled_projects`, nil,
		func(sc scanner) (string, error) {
			var p string
			err := sc.Scan(&p)
			return p, err
		})
	if err != nil {
		return nil, fmt.Errorf("list enrolled projects: %w", err)
	}
	isEnrolled := make(map[string]bool, len(enrolled))
	for _, p := range enrolled {
		isEnrolled[p] = true
	}

	byKey := map[string][]FragmentedProjectName{}
	for _, n := range names {
		k := FragmentKey(n.Name)
		if k == "" {
			continue
		}
		n.Enrolled = isEnrolled[n.Name]
		byKey[k] = append(byKey[k], n)
	}

	groups := []FragmentedGroup{}
	for key, members := range byKey {
		for subKey, sub := range splitFragments(key, members) {
			if len(sub) < 2 {
				continue
			}
			groups = append(groups, newFragmentedGroup(subKey, sub))
		}
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Key < groups[j].Key })
	return groups, nil
}

// splitFragments turns one FragmentKey bucket into groups whose members all
// relate to the group's target. With a plain name in the bucket, every member
// relates to it (path vs its basename, or a folded-equal name), so the bucket
// is one group. Without one, only paths equal after folding relate, so the
// bucket splits by folded name.
func splitFragments(key string, members []FragmentedProjectName) map[string][]FragmentedProjectName {
	for _, m := range members {
		if !isPathName(foldName(m.Name)) {
			return map[string][]FragmentedProjectName{key: members}
		}
	}
	out := map[string][]FragmentedProjectName{}
	for _, m := range members {
		f := foldName(m.Name)
		out[f] = append(out[f], m)
	}
	return out
}

// newFragmentedGroup orders members so the suggested target comes first and
// marks which names can be combined into it.
func newFragmentedGroup(key string, members []FragmentedProjectName) FragmentedGroup {
	sort.Slice(members, func(i, j int) bool { return targetBefore(key, members[i], members[j]) })
	g := FragmentedGroup{Key: key, Target: members[0].Name, TargetEnrolled: members[0].Enrolled}
	for i := range members {
		members[i].Combinable = i > 0 && !members[i].Enrolled && !g.TargetEnrolled && SameFragment(members[i].Name, g.Target)
	}
	g.Names = members
	return g
}

// targetBefore orders group members so the suggested target comes first: a
// plain name before a path (so every path is "path vs its basename" of the
// target), then most memories, most sessions, the name already equal to the
// key, the shortest, and finally by name.
func targetBefore(key string, a, b FragmentedProjectName) bool {
	if pa, pb := isPathName(foldName(a.Name)), isPathName(foldName(b.Name)); pa != pb {
		return !pa
	}
	if a.Memories != b.Memories {
		return a.Memories > b.Memories
	}
	if a.Sessions != b.Sessions {
		return a.Sessions > b.Sessions
	}
	if (a.Name == key) != (b.Name == key) {
		return a.Name == key
	}
	if len(a.Name) != len(b.Name) {
		return len(a.Name) < len(b.Name)
	}
	return a.Name < b.Name
}
