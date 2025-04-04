package slogdedup

import (
	"log/slog"
	"strconv"
)

// JoinResolveKey can be used to join together many slogdedup middlewares
// xHandlerOptions.ResolveKey functions into a single one that applies all the
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
		return incrementKeyName(key, index), ok
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

// ResolveReplaceOptions is a struct of optional options that change the
// behavior of the ResolveKey and ReplaceAttr functions.
type ResolveReplaceOptions struct {
	// OverwriteSummary, if true and applicable to the log sink, will ensure the
	// builtin slog.Record "msg" key will be changed to the appropriate
	// "message" or "summary" key for that sink (usually causing the msg to show
	// up as the log line summary when skimming.
	OverwriteSummary bool
}

// ResolveKeyGraylog returns a ResolveKey function works for Graylog.
// If OverwriteSummary is true, the slog.Record "msg" key will be changed to "message",
// causing it to show up as the main log line when skimming.
func ResolveKeyGraylog(options *ResolveReplaceOptions) func(groups []string, key string, index int) (string, bool) {
	return resolveKeys(sinkGraylog(options))
}

// ReplaceAttrGraylog returns a ReplaceAttr function works for Graylog.
// If OverwriteSummary is true, the slog.Record "msg" key will be changed to "message",
// causing it to show up as the main log line when skimming.
func ReplaceAttrGraylog(options *ResolveReplaceOptions) func(groups []string, a slog.Attr) slog.Attr {
	return replaceAttr(sinkGraylog(options))
}

// Graylog https://graylog.org/
func sinkGraylog(options *ResolveReplaceOptions) sink {
	finalMsgKey := slog.MessageKey
	if options != nil && options.OverwriteSummary {
		// "message" is what Graylog will show when skimming. It defaults to the entire log payload.
		// Have the builtin message use this as its key.
		finalMsgKey = "message"
	}

	return sink{
		// builtins are going to be the FINAL key names for the 4 builtin fields on slog.Record.
		// We will also add in any fields we want incremented, if they would be assigned a special value by graylog.
		// In this case, we want to increment "message" regardless of whether it will be overwritten by the "msg" builtin or not.
		builtins: []string{slog.TimeKey, slog.LevelKey, finalMsgKey, "sourceLoc", "message"},
		replacers: map[string]attrReplacer{
			// "timestamp" is the time of the record. Defaults to the time the log was received by graylog.
			// If using a json extractor or rule, Graylog needs to have it set to a time object, not a string.
			// So best to let your timestamp come in under a different key, then set it specifically with a pipeline rule.
			"timestamp": {key: "timestampRenamed"},

			slog.MessageKey: {key: finalMsgKey},

			// "source" is the IP address or similar of where the logs came from.
			// Let Graylog keep its enchriched field, and rename our source location.
			slog.SourceKey: {key: "sourceLoc"},
		},
	}
}

// ResolveKeyStackdriver returns a ResolveKey function works for Stackdriver
// (aka Google Cloud Operations, aka GCP Log Explorer).
// If OverwriteSummary is true, the slog.Record "msg" key will be changed to "message",
// causing it to show up as the main log line when skimming.
func ResolveKeyStackdriver(options *ResolveReplaceOptions) func(groups []string, key string, index int) (string, bool) {
	return resolveKeys(sinkStackdriver(options))
}

// ReplaceAttrStackdriver returns a ReplaceAttr function works for Stackdriver
// (aka Google Cloud Operations, aka GCP Log Explorer).
// If OverwriteSummary is true, the slog.Record "msg" key will be changed to "message",
// causing it to show up as the main log line when skimming.
func ReplaceAttrStackdriver(options *ResolveReplaceOptions) func(groups []string, a slog.Attr) slog.Attr {
	return replaceAttr(sinkStackdriver(options))
}

// Stackdriver, aka Google Cloud Operations, aka GCP Log Explorer
// https://cloud.google.com/products/operations
func sinkStackdriver(options *ResolveReplaceOptions) sink {
	finalMsgKey := slog.MessageKey
	if options != nil && options.OverwriteSummary {
		// "message" is what Stackdriver will show when skimming. It defaults to the entire log payload.
		// Have the builtin message use this as its key.
		finalMsgKey = "message"
	}

	return sink{
		// builtins are going to be the FINAL key names for the 4 builtin fields on slog.Record.
		// We will also add in any fields we want incremented, if they would be assigned a special value by graylog.
		// In this case, we want to increment "message" regardless of whether it will be overwritten by the "msg" builtin or not.
		builtins: []string{slog.TimeKey, "severity", finalMsgKey, "logging.googleapis.com/sourceLocation", "message"},
		replacers: map[string]attrReplacer{
			// The default slog time key is "time", which stackdriver will detect and parse:
			// https://cloud.google.com/logging/docs/agent/logging/configuration#special-fields

			// "severity" is what Stackdriver uses for the log level:
			// https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#LogSeverity
			// Have the builtin level use this as its key.
			slog.LevelKey: {key: "severity", valuer: func(v slog.Value) slog.Value {
				switch lvl := v.Any().(type) {
				case slog.Level:
					if lvl <= slog.LevelDebug {
						return slog.StringValue("DEBUG") // -4
					} else if lvl <= slog.LevelInfo {
						return slog.StringValue("INFO") // 0
					} else if lvl <= slog.LevelInfo+2 {
						return slog.StringValue("NOTICE") // 2
					} else if lvl <= slog.LevelWarn {
						return slog.StringValue("WARNING") // 4
					} else if lvl <= slog.LevelError {
						return slog.StringValue("ERROR") // 8
					} else if lvl <= slog.LevelError+4 {
						return slog.StringValue("CRITICAL") // 12
					} else if lvl <= slog.LevelError+8 {
						return slog.StringValue("ALERT") // 16
					}
					return slog.StringValue("EMERGENCY")
				default:
					return v
				}
			}},

			slog.MessageKey: {key: finalMsgKey},

			// "logging.googleapis.com/sourceLocation" is what Stackdriver expects for
			// the key containing the file, line, and function values.
			// Have the builtin source use this as its key.
			slog.SourceKey: {key: "logging.googleapis.com/sourceLocation", valuer: func(v slog.Value) slog.Value {
				// https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#LogEntrySourceLocation
				switch source := v.Any().(type) {
				case *slog.Source:
					if source == nil {
						return v
					}
					return slog.AnyValue(struct {
						Function string `json:"function"`
						File     string `json:"file"`
						Line     string `json:"line"` // slog.Source.Line is an int, GCP wants a string
					}{
						Function: source.Function,
						File:     source.File,
						Line:     strconv.Itoa(source.Line),
					})
				default:
					return v
				}
			}},
		},
	}
}

// ReplaceAttrCloudwatch returns a ReplaceAttr function works for Cloudwatch
// (AWS Cloudwatch Logs, Cloudwatch Log Insights, etc).
// https://docs.aws.amazon.com/AmazonCloudWatch/latest/logs/AnalyzingLogData.html
// ResolveReplaceOptions are currently ignored for Cloudwatch.
// Cloudwatch does not need a ResolveKey function at this point.
func ReplaceAttrCloudwatch(_ *ResolveReplaceOptions) func(groups []string, a slog.Attr) slog.Attr {
	// Output the top level time argument with a specific format,
	// Because AWS Cloudwatch sorts time as a string instead of as a time.
	const RFC3339NanoConstantSize = "2006-01-02T15:04:05.000000000Z07:00"
	return func(groups []string, a slog.Attr) slog.Attr {
		if groups == nil && a.Key == slog.TimeKey && a.Value.Kind() == slog.KindTime {
			return slog.String(a.Key, a.Value.Time().Format(RFC3339NanoConstantSize))
		}
		return a
	}
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
// xHandlerOptions.ResolveKey. Its purpose is to replace the key on any
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
	// builtin "source" is logged by the sink it gets replaced with
	// "sourceLoc", ending up with duplicates.
	// Example: slog.Info("main", slog.String(slog.MessageKey, "hello"), slog.String("message", "world"))
	// Should, if using Graylog or Stackdriver, come out as:
	// {"message":"main", "message#01":"hello", "message#02":"world"}
	return func(groups []string, key string, index int) (string, bool) {
		if len(groups) > 0 {
			return key, true
		}

		// Check replacers first. (slog.Record builtin fields are not present, see above comment)
		for oldKey, replacement := range dest.replacers {
			if key == oldKey {
				key = replacement.key
			}
		}

		// Check builtins last. This will rename any regular attributes so that
		// they don't conflict with the builtin fields on slog.Record
		for _, builtin := range dest.builtins {
			if key == builtin {
				return incrementKeyName(key, index+1), true
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

		// This will still catch the builtin fields.
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
