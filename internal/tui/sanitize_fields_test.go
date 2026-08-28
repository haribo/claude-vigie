package tui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
)

// sanitizeSessions cleans the fields it was written to clean, and that is the
// whole trouble: #540 was one field of thirteen left out, #629 was five that did
// not look like text, and #635 was two entire payloads nobody had walked. Each
// time the fix was a hand-kept list, and each time the list was scoped to one
// struct.
//
// These walk the types instead. Every string anywhere in a payload the terminal
// draws — struct field, slice element, map key, map value — is planted with a
// control sequence and must come back clean. A field added tomorrow fails here,
// which is the only reason there is not a fourth instance.

// hostile is what a report can put in any string it controls: an OSC that renames
// the operator's terminal window, and a CSI that clears their screen.
const hostile = "before\x1b]0;pwned\x07\x1b[2Jafter"

// plantHostile returns a copy of v with every string it reaches set to `hostile`.
func plantHostile(v reflect.Value) reflect.Value {
	switch v.Kind() {
	case reflect.String:
		return reflect.ValueOf(hostile).Convert(v.Type())
	case reflect.Struct:
		out := reflect.New(v.Type()).Elem()
		out.Set(v)
		for i := 0; i < v.NumField(); i++ {
			if out.Type().Field(i).IsExported() {
				out.Field(i).Set(plantHostile(v.Field(i)))
			}
		}
		return out
	case reflect.Slice:
		out := reflect.MakeSlice(v.Type(), 1, 1)
		out.Index(0).Set(plantHostile(reflect.New(v.Type().Elem()).Elem()))
		return out
	case reflect.Map:
		out := reflect.MakeMap(v.Type())
		k := plantHostile(reflect.New(v.Type().Key()).Elem())
		out.SetMapIndex(k, plantHostile(reflect.New(v.Type().Elem()).Elem()))
		return out
	case reflect.Array:
		out := reflect.New(v.Type()).Elem()
		for i := 0; i < out.Len(); i++ {
			out.Index(i).Set(plantHostile(out.Index(i)))
		}
		return out
	case reflect.Pointer:
		p := reflect.New(v.Type().Elem())
		p.Elem().Set(plantHostile(p.Elem()))
		return p
	default:
		return v
	}
}

// dirtyPaths names every string still carrying a control character, by the path
// a reader can follow back to the field.
func dirtyPaths(v reflect.Value, path string, out *[]string) {
	switch v.Kind() {
	case reflect.String:
		if strings.ContainsFunc(v.String(), isControl) {
			*out = append(*out, path)
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).IsExported() {
				dirtyPaths(v.Field(i), path+"."+v.Type().Field(i).Name, out)
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			dirtyPaths(v.Index(i), path+"[]", out)
		}
	case reflect.Map:
		for _, k := range v.MapKeys() {
			dirtyPaths(k, path+"{key}", out)
			dirtyPaths(v.MapIndex(k), path+"{}", out)
		}
	case reflect.Pointer:
		if !v.IsNil() {
			dirtyPaths(v.Elem(), path+"*", out)
		}
	}
}

// assertClean checks the sanitized value, and first checks that the planted one
// was dirty at all: every guard below is vacant if `plantHostile` silently walks
// past a kind, and a guard that cannot fail is worse than none.
func assertClean(t *testing.T, name string, before, after any, exempt map[string]string) {
	t.Helper()
	var planted []string
	dirtyPaths(reflect.ValueOf(before), name, &planted)
	if len(planted) == 0 {
		t.Fatalf("%s: nothing was planted — the walk is broken, not the code", name)
	}
	var dirty []string
	dirtyPaths(reflect.ValueOf(after), name, &dirty)
	for _, p := range dirty {
		if why, ok := exempt[p]; ok {
			t.Logf("%s exempt: %s", p, why)
			continue
		}
		t.Errorf("%s reaches the terminal with control characters intact — clean it on the way in, "+
			"or exempt it here with the reason", p)
	}
}

func TestASessionViewIsCleanedThrough(t *testing.T) {
	in := plantHostile(reflect.ValueOf(api.SessionView{})).Interface().(api.SessionView)
	assertClean(t, "SessionView", in, sanitizeSessions([]api.SessionView{in})[0], nil)
}

func TestTheWatcherStatusIsCleanedThrough(t *testing.T) {
	in := plantHostile(reflect.ValueOf(api.WatcherStatus{})).Interface().(api.WatcherStatus)
	assertClean(t, "WatcherStatus", in, sanitizeWatcherStatus(in), nil)
}

func TestTheStatsAreCleanedThrough(t *testing.T) {
	in := plantHostile(reflect.ValueOf(api.StatsResponse{})).Interface().(api.StatsResponse)
	assertClean(t, "StatsResponse", in, sanitizeStats(in), nil)
}

func TestTheDaemonBuildIsCleanedThrough(t *testing.T) {
	in := plantHostile(reflect.ValueOf(api.VersionInfo{})).Interface().(api.VersionInfo)
	assertClean(t, "VersionInfo", in, SanitizeVersion(in), nil)
}
