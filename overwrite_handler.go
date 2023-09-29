package dedup

import (
	"context"
	"log/slog"
	"strings"

	"modernc.org/b/v2"
)

// TODO: also create an append version, and a key name increment version
// TODO: also create a sorting middleware as well
// TODO: also create a pretty json printer that still prints to only 1 line, just prettier

var (
	// TimeKey is the key used by the built-in handlers for the time when the log method is called.
	// Set before instantiating any handlers.
	TimeKey = slog.TimeKey

	// LevelKey is the key used by the built-in handlers for the level of the log call.
	// Set before instantiating any handlers.
	LevelKey = slog.LevelKey

	// MessageKey is the key used by the built-in handlers for the message of the log call.
	// Set before instantiating any handlers.
	MessageKey = slog.MessageKey

	// SourceKey is the key used by the built-in handlers for the source file and line of the log call.
	// Set before instantiating any handlers.
	SourceKey = slog.SourceKey
)

// OverwriteHandlerOptions are options for a OverwriteHandler. An empty options struct is NOT valid.
type OverwriteHandlerOptions struct {
	// Comparison function to determine if two keys are equal
	KeyCompare func(a, b string) int

	// Function that will be called on all root level (not in a group) attribute keys.
	// Returns the new key value to use, and true to keep the attribute or false to drop it.
	// Can be used to drop, keep, or rename any attributes matching the builtin attributes.
	ResolveBuiltinKeyConflict func(k string) (string, bool)
}

// OverwriteHandler is a slog.Handler middleware that will deduplicate all attributes and
// groups by overwriting any older attributes or groups with the same string key.
// It passes the final record and attributes off to the next handler when finished.
type OverwriteHandler struct {
	next       slog.Handler
	goa        *groupOrAttrs
	keyCompare func(a, b string) int
	getKey     func(key string, depth int) (string, bool)
}

var _ slog.Handler = &OverwriteHandler{} // Assert conformance with interface

// NewOverwriteHandler creates a OverwriteHandler slog.Handler middleware that will deduplicate all attributes and
// groups by overwriting any older attributes or groups with the same string key.
// It passes the final record and attributes off to the next handler when finished.
// If opts is nil, the default options are used.
func NewOverwriteHandler(next slog.Handler, opts *OverwriteHandlerOptions) *OverwriteHandler {
	if opts == nil {
		opts = &OverwriteHandlerOptions{}
	}
	if opts.KeyCompare == nil {
		opts.KeyCompare = CaseSensitiveCmp
	}
	if opts.ResolveBuiltinKeyConflict == nil {
		opts.ResolveBuiltinKeyConflict = IncrementIfBuiltinKeyConflict
	}

	return &OverwriteHandler{
		next:       next,
		keyCompare: opts.KeyCompare,
		getKey:     getKeyClosure(opts.ResolveBuiltinKeyConflict),
	}
}

// Enabled reports whether the handler handles records at the given level.
// The handler ignores records whose level is lower.
func (h *OverwriteHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

// Handle de-duplicates all attributes and groups, then passes the new set of attributes to the next handler.
func (h *OverwriteHandler) Handle(ctx context.Context, r slog.Record) error {
	// The final set of attributes on the record, is basically the same as a final With-Attributes groupOrAttrs.
	// So collect all final attributes and turn them into a groupOrAttrs so that it can be handled the same.
	finalAttrs := make([]slog.Attr, 0, r.NumAttrs())
	r.Attrs(func(a slog.Attr) bool {
		finalAttrs = append(finalAttrs, a)
		return true
	})
	goas := collectGroupOrAttrs(h.goa, &groupOrAttrs{attrs: finalAttrs})

	// Resolve groups and with-attributes
	uniq := b.TreeNew[string, any](h.keyCompare)
	createAttrTree(uniq, goas, 0, h.keyCompare, h.getKey)

	// Add all attributes to new record (because old record has all the old attributes)
	newR := &slog.Record{
		Time:    r.Time,
		Level:   r.Level,
		Message: r.Message,
		PC:      r.PC,
	}

	// Add deduplicated attributes back in
	newR.AddAttrs(buildAttrs(uniq)...)
	return h.next.Handle(ctx, *newR)
}

// WithGroup returns a new OverwriteHandler that still has h's attributes,
// but any future attributes added will be namespaced.
func (h *OverwriteHandler) WithGroup(name string) slog.Handler {
	h2 := *h
	h2.goa = h2.goa.WithGroup(name)
	return &h2
}

// WithAttrs returns a new OverwriteHandler whose attributes consists of h's attributes followed by attrs.
func (h *OverwriteHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	h2 := *h
	h2.goa = h2.goa.WithAttrs(attrs)
	return &h2
}

// createAttrTree recursively goes through all groupOrAttrs, resolving their attributes and creating subtrees as
// necessary, adding the results to the map
func createAttrTree(uniq *b.Tree[string, any], goas []*groupOrAttrs, depth int, keyCompare func(a, b string) int, getKey func(key string, depth int) (string, bool)) {
	if len(goas) == 0 {
		return
	}

	// If a group is encountered, create a subtree for that group and all groupOrAttrs after it
	if goas[0].group != "" {
		if key, ok := getKey(goas[0].group, depth); ok {
			uniqGroup := b.TreeNew[string, any](keyCompare)
			createAttrTree(uniqGroup, goas[1:], depth+1, keyCompare, getKey)
			// Ignore empty groups, otherwise put subtree into the map
			if uniqGroup.Len() > 0 {
				uniq.Set(key, uniqGroup)
			}
			return
		}
	}

	// Otherwise, set all attributes for this groupOrAttrs, and then call again for remaining groupOrAttrs's
	resolveValues(uniq, goas[0].attrs, depth, keyCompare, getKey)
	createAttrTree(uniq, goas[1:], depth, keyCompare, getKey)
}

// resolveValues iterates through the attributes, resolving them and putting them into the map.
// If a group is encountered (as an attribute), it will be separately resolved and added as a subtree.
// Since attributes are ordered from oldest to newest, it overwrites keys as it goes.
func resolveValues(uniq *b.Tree[string, any], attrs []slog.Attr, depth int, keyCompare func(a, b string) int, getKey func(key string, depth int) (string, bool)) {
	for _, a := range attrs {
		a.Value = a.Value.Resolve()
		if a.Equal(slog.Attr{}) {
			continue // Ignore empty attributes, and keep iterating
		}

		// Default situation: resolve the key and put it into the map
		key, ok := getKey(a.Key, depth)
		if !ok {
			continue
		}

		if a.Value.Kind() != slog.KindGroup {
			uniq.Set(key, a)
			continue
		}

		// Groups with empty keys are inlined
		if key == "" {
			resolveValues(uniq, a.Value.Group(), depth, keyCompare, getKey)
			continue
		}

		// Create a subtree for this group
		uniqGroup := b.TreeNew[string, any](keyCompare)
		resolveValues(uniqGroup, a.Value.Group(), depth+1, keyCompare, getKey)

		// Ignore empty groups, otherwise put subtree into the map
		if uniqGroup.Len() > 0 {
			uniq.Set(key, uniqGroup)
		}
	}
}

// buildAttrs converts the deduplicated map back into an attribute array, with any subtrees converted into slog.Group's
func buildAttrs(uniq *b.Tree[string, any]) []slog.Attr {
	en, err := uniq.SeekFirst()
	if err != nil {
		return nil // Empty (btree only returns an error when empty)
	}
	defer en.Close()

	// Iterate through all values in the map, add to slice
	attrs := make([]slog.Attr, 0, uniq.Len())
	for k, i, err := en.Next(); err == nil; k, i, err = en.Next() {
		// Values will either be an attribute, or a subtree
		switch v := i.(type) {
		case slog.Attr:
			attrs = append(attrs, v)
		case *b.Tree[string, any]:
			// Convert subtree into a group
			attrs = append(attrs, slog.Attr{Key: k, Value: slog.GroupValue(buildAttrs(v)...)})
		default:
			panic("unexpected type in attribute map")
		}
	}
	return attrs
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
	if key == TimeKey || key == LevelKey || key == MessageKey || key == SourceKey {
		return key + "#01", true // Don't overwrite the built-in attribute keys
	}
	return key, true
}

// DropIfBuiltinKeyConflict will, if there is a conflict/duplication at the root level (not in a group) with one of the
// built-in keys, drop the whole attribute
func DropIfBuiltinKeyConflict(key string) (string, bool) {
	if key == TimeKey || key == LevelKey || key == MessageKey || key == SourceKey {
		return "", false // Drop the attribute
	}
	return key, true
}

// KeepIfBuiltinKeyConflict will keep all keys even if there would be a conflict/duplication at the root level (not in a
// group) with one of the built-in keys
func KeepIfBuiltinKeyConflict(key string) (string, bool) {
	return key, true // Keep all
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
