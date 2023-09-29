package dedup

import (
	"context"
	"log/slog"

	"modernc.org/b/v2"
)

// TODO: also create an append version, and a key name increment version
// TODO: also create a sorting middleware as well
// TODO: also create a pretty json printer that still prints to only 1 line, just prettier

// OverwriteHandlerOptions are options for a OverwriteHandler
type OverwriteHandlerOptions struct {
	// TODO: maybe put the comparison function here?
}

// OverwriteHandler is a slog.Handler middleware that will deduplicate all attributes and
// groups by overwriting any older attributes or groups with the same string key.
// It passes the final record and attributes off to the next handler when finished.
type OverwriteHandler struct {
	next slog.Handler
	opts *OverwriteHandlerOptions
	goa  *groupOrAttrs
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
	return &OverwriteHandler{next: next, opts: opts}
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
	uniq := b.TreeNew[string, any](strCmp)
	createAttrTree(uniq, goas, 0)

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
func createAttrTree(uniq *b.Tree[string, any], goas []*groupOrAttrs, depth int) {
	if len(goas) == 0 {
		return
	}

	// If a group is encountered, create a subtree for that group and all groupOrAttrs after it
	if goas[0].group != "" {
		uniqGroup := b.TreeNew[string, any](strCmp)
		createAttrTree(uniqGroup, goas[1:], depth+1)
		// Ignore empty groups, otherwise put subtree into the map
		if uniqGroup.Len() > 0 {
			uniq.Set(getKey(goas[0].group, depth), uniqGroup)
		}
		return
	}

	// Otherwise, set all attributes for this groupOrAttrs, and then call again for remaining groupOrAttrs's
	resolveValues(uniq, goas[0].attrs, depth)
	createAttrTree(uniq, goas[1:], depth)
}

// getKey resolves a key, making sure not to overwrite the 4 built-in attribute keys (time, level, msg, source).
// TODO: what behavior do we want for the OverwriteHandler when an attribute is using the built-in keys?
// Technically, because the built-in keys are set last, they would always overwrite any attributes with the same key,
// effectively meaning that those attributes never existed.
func getKey(key string, depth int) string {
	if depth == 0 {
		if key == slog.TimeKey || key == slog.LevelKey || key == slog.MessageKey || key == slog.SourceKey {
			return key + "#01" // Don't overwrite the built-in attribute keys
		}
	}
	return key
}

// resolveValues iterates through the attributes, resolving them and putting them into the map.
// If a group is encountered (as an attribute), it will be separately resolved and added as a subtree.
// Since attributes are ordered from oldest to newest, it overwrites keys as it goes.
func resolveValues(uniq *b.Tree[string, any], attrs []slog.Attr, depth int) {
	for _, a := range attrs {
		a.Value = a.Value.Resolve()
		if a.Equal(slog.Attr{}) {
			continue // Ignore empty attributes, and keep iterating
		}

		// Default situation: resolve the key and put it into the map
		a.Key = getKey(a.Key, depth)
		if a.Value.Kind() != slog.KindGroup {
			uniq.Set(a.Key, a)
			continue
		}

		// Groups with empty keys are inlined
		if a.Key == "" {
			resolveValues(uniq, a.Value.Group(), depth)
			continue
		}

		// Create a subtree for this group
		uniqGroup := b.TreeNew[string, any](strCmp)
		resolveValues(uniqGroup, a.Value.Group(), depth+1)

		// Ignore empty groups, otherwise put subtree into the map
		if uniqGroup.Len() > 0 {
			uniq.Set(a.Key, uniqGroup)
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

func strCmp(a, b string) int {
	if a == b {
		return 0
	}
	if a > b {
		return 1
	}
	return -1
}
