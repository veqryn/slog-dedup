package slogdedup

import (
	"context"
	"log/slog"
	"slices"

	"modernc.org/b/v2"
)

// AppendHandlerOptions are options for a AppendHandler
type AppendHandlerOptions struct {
	// Comparison function to determine if two keys are equal
	KeyCompare func(a, b string) int

	// Function that will be called on all root level (not in a group) attribute keys.
	// Returns the new key value to use, and true to keep the attribute or false to drop it.
	// Can be used to drop, keep, or rename any attributes matching the builtin attributes.
	ResolveKey func(groups []string, key string, _ int) (string, bool)
}

// AppendHandler is a slog.Handler middleware that will deduplicate all attributes and
// groups by creating a slice/array whenever there is more than one attribute with the same key.
// It passes the final record and attributes off to the next handler when finished.
type AppendHandler struct {
	next       slog.Handler
	goa        *groupOrAttrs
	keyCompare func(a, b string) int
	resolveKey func(groups []string, key string, _ int) (string, bool)
}

var _ slog.Handler = &AppendHandler{} // Assert conformance with interface

// NewAppendMiddleware creates an AppendHandler slog.Handler middleware
// that conforms to [github.com/samber/slog-multi.Middleware] interface.
// It can be used with slogmulti methods such as Pipe to easily setup a pipeline of slog handlers:
//
//	slog.SetDefault(slog.New(slogmulti.
//		Pipe(slogcontext.NewMiddleware(&slogcontext.HandlerOptions{})).
//		Pipe(slogdedup.NewAppendMiddleware(&slogdedup.AppendHandlerOptions{})).
//		Handler(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{})),
//	))
func NewAppendMiddleware(options *AppendHandlerOptions) func(slog.Handler) slog.Handler {
	return func(next slog.Handler) slog.Handler {
		return NewAppendHandler(
			next,
			options,
		)
	}
}

// NewAppendHandler creates a AppendHandler slog.Handler middleware that will deduplicate all attributes and
// groups by creating a slice/array whenever there is more than one attribute with the same key.
// It passes the final record and attributes off to the next handler when finished.
// If opts is nil, the default options are used.
func NewAppendHandler(next slog.Handler, opts *AppendHandlerOptions) *AppendHandler {
	if opts == nil {
		opts = &AppendHandlerOptions{}
	}
	if opts.KeyCompare == nil {
		opts.KeyCompare = CaseSensitiveCmp
	}
	if opts.ResolveKey == nil {
		opts.ResolveKey = IncrementIfBuiltinKeyConflict
	}

	return &AppendHandler{
		next:       next,
		keyCompare: opts.KeyCompare,
		resolveKey: opts.ResolveKey,
	}
}

// Enabled reports whether the next handler handles records at the given level.
// The handler ignores records whose level is lower.
func (h *AppendHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

// Handle de-duplicates all attributes and groups, then passes the new set of attributes to the next handler.
func (h *AppendHandler) Handle(ctx context.Context, r slog.Record) error {
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
	h.createAttrTree(uniq, goas, nil)

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

// WithGroup returns a new AppendHandler that still has h's attributes,
// but any future attributes added will be namespaced.
func (h *AppendHandler) WithGroup(name string) slog.Handler {
	h2 := *h
	h2.goa = h2.goa.WithGroup(name)
	return &h2
}

// WithAttrs returns a new AppendHandler whose attributes consists of h's attributes followed by attrs.
func (h *AppendHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	h2 := *h
	h2.goa = h2.goa.WithAttrs(attrs)
	return &h2
}

// createAttrTree recursively goes through all groupOrAttrs, resolving their attributes and creating subtrees as
// necessary, adding the results to the map
func (h *AppendHandler) createAttrTree(uniq *b.Tree[string, any], goas []*groupOrAttrs, groups []string) {
	if len(goas) == 0 {
		return
	}

	// If a group is encountered, create a subtree for that group and all groupOrAttrs after it
	if goas[0].group != "" {
		if key, keep := h.resolveKey(groups, goas[0].group, 0); keep {
			uniqGroup := b.TreeNew[string, any](h.keyCompare)
			h.createAttrTree(uniqGroup, goas[1:], append(slices.Clip(groups), key))
			// Ignore empty groups, otherwise put subtree into the map
			if uniqGroup.Len() > 0 {
				// Put calls func(oldValue, true) if key already exists, or func(oldValue, false) if it doesn't.
				// Then expects us to return (newValue, true) if replacing the oldValue, or (whatever, false) if not.
				uniq.Put(key, func(oldValue any, exists bool) (any, bool) {
					if !exists {
						return uniqGroup, true
					}
					if slice, ok := oldValue.(appended); ok {
						slice = append(slice, uniqGroup)
						return slice, true
					}
					return appended{oldValue, uniqGroup}, true
				})
			}
			return
		}
	}

	// Otherwise, set all attributes for this groupOrAttrs, and then call again for remaining groupOrAttrs's
	h.resolveValues(uniq, goas[0].attrs, groups)
	h.createAttrTree(uniq, goas[1:], groups)
}

// resolveValues iterates through the attributes, resolving them and putting them into the map.
// If a group is encountered (as an attribute), it will be separately resolved and added as a subtree.
// Since attributes are ordered from oldest to newest, it creates a slice whenever it detects the key already exists,
// appending the new attribute, then overwriting the key with that slice.
func (h *AppendHandler) resolveValues(uniq *b.Tree[string, any], attrs []slog.Attr, groups []string) {
	var keep bool
	for _, a := range attrs {
		a.Value = a.Value.Resolve()
		if a.Equal(slog.Attr{}) {
			continue // Ignore empty attributes, and keep iterating
		}

		// Default situation: resolve the key and put it into the map
		a.Key, keep = h.resolveKey(groups, a.Key, 0)
		if !keep {
			continue
		}

		if a.Value.Kind() != slog.KindGroup {
			uniq.Put(a.Key, func(oldValue any, exists bool) (any, bool) {
				if !exists {
					return a, true
				}
				if slice, ok := oldValue.(appended); ok {
					slice = append(slice, a)
					return slice, true
				}
				return appended{oldValue, a}, true
			})
			continue
		}

		// Groups with empty keys are inlined
		if a.Key == "" {
			h.resolveValues(uniq, a.Value.Group(), groups)
			continue
		}

		// Create a subtree for this group
		uniqGroup := b.TreeNew[string, any](h.keyCompare)
		h.resolveValues(uniqGroup, a.Value.Group(), append(slices.Clip(groups), a.Key))

		// Ignore empty groups, otherwise put subtree into the map
		if uniqGroup.Len() > 0 {
			uniq.Put(a.Key, func(oldValue any, exists bool) (any, bool) {
				if !exists {
					return uniqGroup, true
				}
				if slice, ok := oldValue.(appended); ok {
					slice = append(slice, uniqGroup)
					return slice, true
				}
				return appended{oldValue, uniqGroup}, true
			})
		}
	}
}
