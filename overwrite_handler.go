package slogdedup

import (
	"context"
	"log/slog"

	"modernc.org/b/v2"
)

// OverwriteHandlerOptions are options for a OverwriteHandler
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

// NewOverwriteMiddleware creates an OverwriteHandler slog.Handler middleware
// that conforms to [github.com/samber/slog-multi.Middleware] interface.
// It can be used with slogmulti methods such as Pipe to easily setup a pipeline of slog handlers:
//
//	slog.SetDefault(slog.New(slogmulti.
//		Pipe(slogcontext.NewMiddleware(&slogcontext.HandlerOptions{})).
//		Pipe(slogdedup.NewOverwriteMiddleware(&slogdedup.OverwriteHandlerOptions{})).
//		Handler(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{})),
//	))
func NewOverwriteMiddleware(options *OverwriteHandlerOptions) func(slog.Handler) slog.Handler {
	return func(next slog.Handler) slog.Handler {
		return NewOverwriteHandler(
			next,
			options,
		)
	}
}

// NewOverwriteHandler creates an OverwriteHandler slog.Handler middleware that will deduplicate all attributes and
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

// Enabled reports whether the next handler handles records at the given level.
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
func (h *OverwriteHandler) createAttrTree(uniq *b.Tree[string, any], goas []*groupOrAttrs, depth int) {
	if len(goas) == 0 {
		return
	}

	// If a group is encountered, create a subtree for that group and all groupOrAttrs after it
	if goas[0].group != "" {
		if key, ok := h.getKey(goas[0].group, depth); ok {
			uniqGroup := b.TreeNew[string, any](h.keyCompare)
			h.createAttrTree(uniqGroup, goas[1:], depth+1)
			// Ignore empty groups, otherwise put subtree into the map
			if uniqGroup.Len() > 0 {
				uniq.Set(key, uniqGroup)
			}
			return
		}
	}

	// Otherwise, set all attributes for this groupOrAttrs, and then call again for remaining groupOrAttrs's
	h.resolveValues(uniq, goas[0].attrs, depth)
	h.createAttrTree(uniq, goas[1:], depth)
}

// resolveValues iterates through the attributes, resolving them and putting them into the map.
// If a group is encountered (as an attribute), it will be separately resolved and added as a subtree.
// Since attributes are ordered from oldest to newest, it overwrites keys as it goes.
func (h *OverwriteHandler) resolveValues(uniq *b.Tree[string, any], attrs []slog.Attr, depth int) {
	var ok bool
	for _, a := range attrs {
		a.Value = a.Value.Resolve()
		if a.Equal(slog.Attr{}) {
			continue // Ignore empty attributes, and keep iterating
		}

		// Default situation: resolve the key and put it into the map
		a.Key, ok = h.getKey(a.Key, depth)
		if !ok {
			continue
		}

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
