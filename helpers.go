package slogdedup

import (
	"fmt"
	"log/slog"
	"strings"

	"modernc.org/b/v2"
)

// TODO: also create a sorting middleware as well
// TODO: also create a pretty json printer that still prints to only 1 line, just prettier

// getKeyClosure returns a function to be used to resolve a key at the root level, determining its behavior when it
// would otherwise conflict/duplicate the 4 built-in attribute keys (time, level, msg, source).
func getKeyClosure(resolveBuiltinKeyConflict func(k string) (string, bool)) func(key string, depth int) (string, bool) {
	return func(key string, depth int) (string, bool) {
		if depth == 0 {
			return resolveBuiltinKeyConflict(key)
		}
		return key, true
	}
}

// IncrementIfBuiltinKeyConflict will, if there is a conflict/duplication at the root level (not in a group) with one of
// the built-in keys, add "#01" to the end of the key
func IncrementIfBuiltinKeyConflict(key string) (string, bool) {
	if DoesBuiltinKeyConflict(key) {
		return IncrementKeyName(key, 1), true // Don't overwrite the built-in attribute keys
	}
	return key, true
}

// DropIfBuiltinKeyConflict will, if there is a conflict/duplication at the root level (not in a group) with one of the
// built-in keys, drop the whole attribute
func DropIfBuiltinKeyConflict(key string) (string, bool) {
	if DoesBuiltinKeyConflict(key) {
		return "", false // Drop the attribute
	}
	return key, true
}

// KeepIfBuiltinKeyConflict will keep all keys even if there would be a conflict/duplication at the root level (not in a
// group) with one of the built-in keys
func KeepIfBuiltinKeyConflict(key string) (string, bool) {
	return key, true // Keep all
}

// DoesBuiltinKeyConflict returns true if the key conflicts with the builtin keys.
// This will only be called on all root level (not in a group) attribute keys.
func DoesBuiltinKeyConflict(key string) bool {
	if key == slog.TimeKey || key == slog.LevelKey || key == slog.MessageKey || key == slog.SourceKey {
		return true
	}
	return false
}

// IncrementKeyName adds a count onto the key name after the first seen.
// Example: keyname, keyname#01, keyname#02, keyname#03
func IncrementKeyName(key string, index int) string {
	if index == 0 {
		return key
	}
	return fmt.Sprintf("%s#%02d", key, index)
}

// CaseSensitiveCmp is a case-sensitive comparison and ordering function that orders by byte values
func CaseSensitiveCmp(a, b string) int {
	if a == b {
		return 0
	}
	if a > b {
		return 1
	}
	return -1
}

// CaseInsensitiveCmp is a case-insensitive comparison and ordering function that orders by byte values
func CaseInsensitiveCmp(a, b string) int {
	a = strings.ToLower(a)
	b = strings.ToLower(b)
	if a == b {
		return 0
	}
	if a > b {
		return 1
	}
	return -1
}

// appended is a type that exists to allow us to differentiate between a log attribute that is a slice or any's ([]any),
// versus when we are appending to the key so that it becomes a slice. Only used with the AppendHandler.
type appended []any

// buildAttrs converts the deduplicated map back into an attribute array,
// with any subtrees converted into slog.Group's
func buildAttrs(uniq *b.Tree[string, any]) []slog.Attr {
	en, err := uniq.SeekFirst()
	if err != nil {
		return nil // Empty (btree only returns an error when empty)
	}
	defer en.Close()

	// Iterate through all values in the map, add to slice
	attrs := make([]slog.Attr, 0, uniq.Len())
	for k, i, err := en.Next(); err == nil; k, i, err = en.Next() {
		// Values will either be an attribute, a subtree, or a specially appended slice of the former two
		switch v := i.(type) {
		case slog.Attr:
			attrs = append(attrs, v)
		case *b.Tree[string, any]:
			// Convert subtree into a group
			attrs = append(attrs, slog.Attr{Key: k, Value: slog.GroupValue(buildAttrs(v)...)})
		case appended:
			// This case only happens in the AppendHandler
			anys := make([]any, 0, len(v))
			for _, sliceVal := range v {
				switch sliceV := sliceVal.(type) {
				case slog.Attr:
					anys = append(anys, sliceV.Value.Any())
				case *b.Tree[string, any]:
					// Convert subtree into a map (because having a Group Attribute within a slice doesn't render)
					anys = append(anys, buildGroupMap(buildAttrs(sliceV)))
				default:
					panic("unexpected type in attribute map")
				}
			}
			attrs = append(attrs, slog.Any(k, anys))
		default:
			panic("unexpected type in attribute map")
		}
	}
	return attrs
}

// buildGroupMap takes a slice of attributes (the attributes within a group), and turns them into a map of string keys
// to a non-attribute resolved value (any).
// This function exists solely to deal with groups that are inside appended-slices (for the AppendHandler),
// because slog does not have a "slice" kind, which means that those groups and their values do not render at all.
func buildGroupMap(attrs []slog.Attr) map[string]any {
	group := map[string]any{}
	for _, attr := range attrs {
		if attr.Value.Kind() != slog.KindGroup {
			group[attr.Key] = attr.Value.Any()
		} else {
			group[attr.Key] = buildGroupMap(attr.Value.Group())
		}
	}
	return group
}

// groupOrAttrs holds either a group name or a list of slog.Attrs.
// It also holds a reference/link to its parent groupOrAttrs, forming a linked list.
type groupOrAttrs struct {
	group string        // group name if non-empty
	attrs []slog.Attr   // attrs if non-empty
	next  *groupOrAttrs // parent
}

// WithGroup returns a new groupOrAttrs that includes the given group, and links to the old groupOrAttrs.
// Safe to call on a nil groupOrAttrs.
func (g *groupOrAttrs) WithGroup(name string) *groupOrAttrs {
	// Empty-name groups are inlined as if they didn't exist
	if name == "" {
		return g
	}
	return &groupOrAttrs{
		group: name,
		next:  g,
	}
}

// WithAttrs returns a new groupOrAttrs that includes the given attrs, and links to the old groupOrAttrs.
// Safe to call on a nil groupOrAttrs.
func (g *groupOrAttrs) WithAttrs(attrs []slog.Attr) *groupOrAttrs {
	if len(attrs) == 0 {
		return g
	}
	return &groupOrAttrs{
		attrs: attrs,
		next:  g,
	}
}

// collectGroupOrAttrs unrolls all individual groupOrAttrs and collects them into a slice, ordered from oldest to newest
func collectGroupOrAttrs(gs ...*groupOrAttrs) []*groupOrAttrs {
	// Get a total count of all groups in the group linked-list chain
	n := 0
	for _, g := range gs {
		for ga := g; ga != nil; ga = ga.next {
			n++
		}
	}

	// The groupOrAttrs on the handler is a linked list starting from the newest to the oldest set of attributes/groups.
	// Within each groupOrAttrs, all attributes are in a slice that is ordered from oldest to newest.
	// To make things consistent we will reverse the order of the groupOrAttrs, so that it goes from oldest to newest,
	// thereby matching the order of the attributes.
	res := make([]*groupOrAttrs, n)
	j := 0
	for i := len(gs) - 1; i >= 0; i-- {
		for ga := gs[i]; ga != nil; ga = ga.next {
			res[len(res)-j-1] = ga
			j++
		}
	}
	return res
}
