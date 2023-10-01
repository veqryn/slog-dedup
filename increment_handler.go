package dedup

import (
	"context"
	"log/slog"

	"modernc.org/b/v2"
)

// IncrementHandlerOptions are options for a IncrementHandler
type IncrementHandlerOptions struct {
	// Comparison function to determine if two keys are equal
	KeyCompare func(a, b string) int

	// Function that will only be called on all root level (not in a group) attribute keys.
	// Returns true if the key conflicts with the builtin keys.
	DoesBuiltinKeyConflict func(key string) bool

	// IncrementKeyName should return a modified key string based on the index (first, second, third instance seen, etc)
	IncrementKeyName func(key string, index int) string
}

// IncrementHandler is a slog.Handler middleware that will deduplicate all attributes and
// groups by incrementing/modifying their key names.
// It passes the final record and attributes off to the next handler when finished.
type IncrementHandler struct {
	next            slog.Handler
	goa             *groupOrAttrs
	keyCompare      func(a, b string) int
	getIncrementKey func(uniq *b.Tree[string, any], depth int, key string) string
}

var _ slog.Handler = &IncrementHandler{} // Assert conformance with interface

// NewIncrementHandler creates a IncrementHandler slog.Handler middleware that will deduplicate all attributes and
// groups by incrementing/modifying their key names.
// It passes the final record and attributes off to the next handler when finished.
// If opts is nil, the default options are used.
func NewIncrementHandler(next slog.Handler, opts *IncrementHandlerOptions) *IncrementHandler {
	if opts == nil {
		opts = &IncrementHandlerOptions{}
	}
	if opts.KeyCompare == nil {
		opts.KeyCompare = CaseSensitiveCmp
	}
	if opts.DoesBuiltinKeyConflict == nil {
		opts.DoesBuiltinKeyConflict = DoesBuiltinKeyConflict
	}
	if opts.IncrementKeyName == nil {
		opts.IncrementKeyName = IncrementKeyName
	}

	return &IncrementHandler{
		next:            next,
		keyCompare:      opts.KeyCompare,
		getIncrementKey: seekIncrementKeyClosure(opts.DoesBuiltinKeyConflict, opts.IncrementKeyName),
	}
}

// Enabled reports whether the next handler handles records at the given level.
// The handler ignores records whose level is lower.
func (h *IncrementHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

// Handle de-duplicates all attributes and groups, then passes the new set of attributes to the next handler.
func (h *IncrementHandler) Handle(ctx context.Context, r slog.Record) error {
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
	h.createAttrTree(uniq, goas, 0)

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

// WithGroup returns a new IncrementHandler that still has h's attributes,
// but any future attributes added will be namespaced.
func (h *IncrementHandler) WithGroup(name string) slog.Handler {
	h2 := *h
	h2.goa = h2.goa.WithGroup(name)
	return &h2
}

// WithAttrs returns a new IncrementHandler whose attributes consists of h's attributes followed by attrs.
func (h *IncrementHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	h2 := *h
	h2.goa = h2.goa.WithAttrs(attrs)
	return &h2
}

// createAttrTree recursively goes through all groupOrAttrs, resolving their attributes and creating subtrees as
// necessary, adding the results to the map
func (h *IncrementHandler) createAttrTree(uniq *b.Tree[string, any], goas []*groupOrAttrs, depth int) {
	if len(goas) == 0 {
		return
	}

	// If a group is encountered, create a subtree for that group and all groupOrAttrs after it
	if goas[0].group != "" {
		goas[0].group = h.getIncrementKey(uniq, depth, goas[0].group)
		uniqGroup := b.TreeNew[string, any](h.keyCompare)
		h.createAttrTree(uniqGroup, goas[1:], depth+1)
		// Ignore empty groups, otherwise put subtree into the map
		if uniqGroup.Len() > 0 {
			uniq.Set(goas[0].group, uniqGroup)
		}
		return
	}

	// Otherwise, set all attributes for this groupOrAttrs, and then call again for remaining groupOrAttrs's
	h.resolveValues(uniq, goas[0].attrs, depth)
	h.createAttrTree(uniq, goas[1:], depth)
}

// resolveValues iterates through the attributes, resolving them and putting them into the map.
// If a group is encountered (as an attribute), it will be separately resolved and added as a subtree.
// Since attributes are ordered from oldest to newest, it increments the key names as it goes.
func (h *IncrementHandler) resolveValues(uniq *b.Tree[string, any], attrs []slog.Attr, depth int) {
	for _, a := range attrs {
		a.Value = a.Value.Resolve()
		if a.Equal(slog.Attr{}) {
			continue // Ignore empty attributes, and keep iterating
		}

		// Default situation: resolve the key and put it into the map
		a.Key = h.getIncrementKey(uniq, depth, a.Key)

		if a.Value.Kind() != slog.KindGroup {
			uniq.Set(a.Key, a)
			continue
		}

		// Groups with empty keys are inlined
		if a.Key == "" {
			h.resolveValues(uniq, a.Value.Group(), depth)
			continue
		}

		// Create a subtree for this group
		uniqGroup := b.TreeNew[string, any](h.keyCompare)
		h.resolveValues(uniqGroup, a.Value.Group(), depth+1)

		// Ignore empty groups, otherwise put subtree into the map
		if uniqGroup.Len() > 0 {
			uniq.Set(a.Key, uniqGroup)
		}
	}
}

// seekIncrementKeyClosure returns a function to be used to resolve a key for IncrementHandler.
func seekIncrementKeyClosure(doesBuiltinKeyConflict func(key string) bool, incrementKeyName func(key string, index int) string) func(uniq *b.Tree[string, any], depth int, key string) string {
	return func(uniq *b.Tree[string, any], depth int, key string) string {
		var index int
		if depth == 0 && doesBuiltinKeyConflict(key) {
			index++ // Don't overwrite the built-in attribute keys
		}

		newKey := incrementKeyName(key, index)

		// Seek cursor to the key in the map equal to or less than newKey
		en, _ := uniq.Seek(newKey)
		defer en.Close()

		// If the next key is missing (io.EOF) or is greater than newKey, return newKey
		for {
			k, _, err := en.Next()
			if err != nil || k > newKey {
				return newKey
			}
			if k == newKey {
				// If the next key is equal to newKey, we must increment our key
				index++
				newKey = incrementKeyName(key, index)
			}
		}
	}
}
