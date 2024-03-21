package slogdedup

import (
	"fmt"
	"log/slog"
	"strconv"
)

// JoinResolveKey can be used to join together many slogdedup middlewares
// ...HandlerOptions.ResolveKey functions into a single one that applies all the
// rules in order.
func JoinResolveKey(resolveKeyFunctions ...func(groups []string, key string, index int) (string, bool)) func(groups []string, key string, index int) (string, bool) {
	if len(resolveKeyFunctions) == 0 {
		return nil
	}
	return func(groups []string, originalKey string, index int) (string, bool) {
		var ok bool
		key := originalKey
		for _, f := range resolveKeyFunctions {
			if key, ok = f(groups, key, index); !ok {
				break
			}
		}
		// Only increment once, and only if the key was not changed.
		// This would happen if we have multiple duplicate keys in row.
		if key != originalKey {
			return key, ok
		}
		return IncrementKeyName(key, index), ok
	}
}

// JoinReplaceAttr can be used to join together many slog.HandlerOptions.ReplaceAttr
// into a single one that applies all rules in order.
func JoinReplaceAttr(replaceAttrFunctions ...func(groups []string, a slog.Attr) slog.Attr) func(groups []string, a slog.Attr) slog.Attr {
	if len(replaceAttrFunctions) == 0 {
		return nil
	}
	return func(groups []string, a slog.Attr) slog.Attr {
		for _, f := range replaceAttrFunctions {
			if a = f(groups, a); a.Equal(slog.Attr{}) {
				break
			}
		}
		return a
	}
}

// ResolveKeyGraylog returns a ResolveKey function works for Graylog.
func ResolveKeyGraylog() func(groups []string, key string, index int) (string, bool) {
	return resolveKeys(sinkGraylog)
}

// ReplaceAttrGraylog returns a ReplaceAttr function works for Graylog.
func ReplaceAttrGraylog() func(groups []string, a slog.Attr) slog.Attr {
	return replaceAttr(sinkGraylog)
}

// Graylog https://graylog.org/
var sinkGraylog = sink{
	builtins: []string{slog.TimeKey, slog.LevelKey, "message", "sourceLoc"},
	replacers: map[string]attrReplacer{
		// "timestamp" is the time of the record. Defaults to the time the log was received by grayload.
		// If using a json extractor or rule, Graylog needs to have it set to a time object, not a string.
		// So best to let your timestamp come in under a different key, then set it specifically with a pipeline rule.
		"timestamp": {key: "timestampRenamed"},

		// "message" is what Graylog will show when skimming. It defaults to the entire log payload.
		// Have the builtin message use this as its key.
		slog.MessageKey: {key: "message"},

		// "source" is the IP address or similar of where the logs came from.
		// Let Graylog keep its enchriched field, and rename our source location.
		slog.SourceKey: {key: "sourceLoc"},
	},
}



// sink represents the final destination of the logs.
type sink struct {
	// Only the keys that will be used for the builtins:
	// (slog.TimeKey, slog.LevelKey, slog.MessageKey, slog.SourceKey)
	builtins []string

	// Replacement key name and optional function to replace the value.
	replacers map[string]attrReplacer
}

// attrReplacer has the replacement key name, and optional function to replace the value
type attrReplacer struct {
	key    string
	valuer func(v slog.Value) slog.Value
}

// resolveKeys returns a closure that can be used with any slogdedup middlewares
// ...HandlerOptions.ResolveKey. Its purpose is to replace the key on any
// attributes or groups, except for the builtin attributes. Using replaceAttr on
// the final handler/sink is still required, in order to replace the builtin
// attribute keys.
func resolveKeys(dest sink) func(groups []string, key string, index int) (string, bool) {
	// This function is for the dedup middlewares.
	// These middlewares do not send the builtin's (time, level, msg, source),
	// because they have no control over the keys that will be used.
	// Only the final/sink handler knows what keys will be used.
	// So avoid situations like "source", where we might have an added
	// field already named "sourceLoc", and then later when the
	// builtin "source" is logged by the sink it get replaced with
	// "sourceLoc", ending up with duplicates.
	// Example: slog.Info("main", slog.String(slog.MessageKey, "hello"), slog.String("message", "world"))
	// Should, if using Graylog or Stackdriver, come out as:
	// {"message":"main", "message#01":"hello", "message#02":"world"}
	return func(groups []string, key string, index int) (string, bool) {
		if len(groups) > 0 {
			return key, true
		}

		// Check replacers first
		for oldKey, replacement := range dest.replacers {
			if key == oldKey {
				key = replacement.key
			}
		}

		// Check builtins last
		for _, builtin := range dest.builtins {
			if key == builtin {
				return IncrementKeyName(key, index+1), true
			}
		}
		return key, true
	}
}

// replaceAttr returns a closure that can be used with slog.HandlerOptions.ReplaceAttr.
// Its purpose is to replace the builtin keys and values only.
// All non-builtin attributes will have their keys modified by resolveKeys.
func replaceAttr(dest sink) func(groups []string, a slog.Attr) slog.Attr {
	// This function is for the final handler (the sink).
	// It knows what keys will be used for the builtin's (time, level, msg, source),
	// and has the ability to modify those keys (and values) here.
	// Standard library sinks, like slog.JSONHandler, do not have the ability to
	// modify the groups at this point, hence why we are modifying them in the
	// resolveKeys function on the dedup middleware instead.
	return func(groups []string, a slog.Attr) slog.Attr {
		if len(groups) > 0 {
			return a
		}

		for oldKey, replacement := range dest.replacers {
			if a.Key == oldKey {
				a.Key = replacement.key
				if replacement.valuer != nil {
					a.Value = replacement.valuer(a.Value)
				}
				return a
			}
		}
		return a
	}
}
