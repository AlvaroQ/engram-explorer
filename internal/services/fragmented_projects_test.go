package services_test

import (
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/services"
)

// TestSameFragment pins the conservative heuristics: case, surrounding
// whitespace, trailing separators and "path-as-name vs its basename" fold
// together; any other prefix/suffix difference does not, and neither do two
// different paths that merely share their last folder.
func TestSameFragment(t *testing.T) {
	cases := []struct {
		a, b string
		same bool
	}{
		{"foo", "Foo", true},
		{"foo", "FOO", true},
		{" foo ", "foo", true},
		{"foo/", "foo", true},
		{"foo\\", "foo", true},
		{"/Users/x/repo/foo", "foo", true},
		{"/Users/x/repo/foo/", "Foo", true},
		{`C:\code\foo`, "foo", true},
		{"e:/Work/Engram Explorer", "engram explorer", true},
		{"/a/foo", "/A/foo/", true},
		{"/a/foo", "/b/foo", false}, // two repos with the same folder name
		// False-positive guards: no generic prefix/suffix matching.
		{"api", "API-docs", false},
		{"foo", "foo-app", false},
		{"foo", "foobar", false},
		{"foo-app", "app", false},
		{"engram", "engram-explorer", false},
		{"/Users/x/foo/bar", "foo", false},
	}
	for _, tc := range cases {
		for _, p := range [][2]string{{tc.a, tc.b}, {tc.b, tc.a}} {
			if got := services.SameFragment(p[0], p[1]); got != tc.same {
				t.Errorf("SameFragment(%q, %q) = %v, want %v", p[0], p[1], got, tc.same)
			}
		}
	}
	for _, empty := range []string{"", "   ", "/", `\\`} {
		if services.SameFragment(empty, empty) {
			t.Errorf("SameFragment(%q, %q) = true, want false (never grouped)", empty, empty)
		}
	}
}

// TestFindFragmentedProjects groups the seeded names, counts memories and
// sessions, picks the name with most memories as target, and marks enrolled
// projects as not combinable.
func TestFindFragmentedProjects(t *testing.T) {
	db, _ := newWriteDB(t)
	stmts := []string{
		`INSERT INTO sessions (id, project, directory) VALUES ('s1', 'foo', '/tmp'), ('s2', '/Users/x/repo/foo', '/tmp'), ('s3', 'Foo', '/tmp'),
		 ('s4', 'api', '/tmp'), ('s5', 'API-docs', '/tmp'), ('s6', 'synced', '/tmp'), ('s7', 'Synced', '/tmp'),
		 ('s8', '/work/lib', '/tmp'), ('s9', '/home/lib', '/tmp'),
		 ('s10', '/x/bar', '/tmp'), ('s11', 'bar', '/tmp')`,
		// "/x/bar" has more memories, but the plain name "bar" is the target:
		// every path member is then directly "path vs its basename" of it.
		`INSERT INTO observations (session_id, type, title, content, project) VALUES
		 ('s10', 'note', 'f', 'c', '/x/bar'), ('s10', 'note', 'g', 'c', '/x/bar'), ('s11', 'note', 'h', 'c', 'bar')`,
		`INSERT INTO observations (session_id, type, title, content, project) VALUES
		 ('s1', 'note', 'a', 'c', 'foo'), ('s1', 'note', 'b', 'c', 'foo'),
		 ('s2', 'note', 'c', 'c', '/Users/x/repo/foo'),
		 ('s6', 'note', 'd', 'c', 'synced')`,
		// A soft-deleted memory does not count.
		`INSERT INTO observations (session_id, type, title, content, project, deleted_at) VALUES ('s3', 'note', 'e', 'c', 'Foo', datetime('now'))`,
		`INSERT INTO sync_enrolled_projects (project) VALUES ('synced')`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	groups, err := services.FindFragmentedProjects(db)
	if err != nil {
		t.Fatalf("FindFragmentedProjects: %v", err)
	}
	if len(groups) != 3 {
		t.Fatalf("got %d groups (%+v), want 3 (bar, foo, synced); api/API-docs and /work/lib vs /home/lib must not group", len(groups), groups)
	}
	byKey := map[string]services.FragmentedGroup{}
	for _, g := range groups {
		byKey[g.Key] = g
	}

	foo := byKey["foo"]
	if foo.Target != "foo" || foo.TargetEnrolled || len(foo.Names) != 3 {
		t.Fatalf("foo group = %+v, want target foo, 3 names, not enrolled", foo)
	}
	want := map[string][2]int{"foo": {2, 1}, "/Users/x/repo/foo": {1, 1}, "Foo": {0, 1}}
	for _, n := range foo.Names {
		w, ok := want[n.Name]
		if !ok || n.Memories != w[0] || n.Sessions != w[1] {
			t.Errorf("foo member %+v, want memories/sessions %v", n, w)
		}
		if n.Combinable != (n.Name != "foo") {
			t.Errorf("foo member %q: combinable=%v", n.Name, n.Combinable)
		}
	}

	bar := byKey["bar"]
	if bar.Target != "bar" || len(bar.Names) != 2 || !bar.Names[1].Combinable || bar.Names[1].Name != "/x/bar" {
		t.Errorf("bar group = %+v, want plain target bar with /x/bar combinable", bar)
	}

	synced := byKey["synced"]
	if synced.Target != "synced" || !synced.TargetEnrolled {
		t.Fatalf("synced group = %+v, want enrolled target synced", synced)
	}
	for _, n := range synced.Names {
		if n.Combinable {
			t.Errorf("%q must not be combinable into an enrolled target", n.Name)
		}
		if n.Enrolled != (n.Name == "synced") {
			t.Errorf("%q: enrolled=%v", n.Name, n.Enrolled)
		}
	}
}
