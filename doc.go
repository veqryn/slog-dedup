/*
Package slogdedup provides structured logging (slog) deduplication for use with json logging
(or any other format where duplicates are not appreciated).
The main impetus behind this package is because most JSON tools do not like duplicate keys for their member
properties/fields. Some of them will give errors or fail to parse the log line, and some may even crash.
Unfortunately the default behavior of the stdlib slog handlers is to allow duplicate keys.

Additionally, this library includes convenience methods for formatting output to
match what is expected for various log aggregation tools (such as Graylog), as
well as cloud providers (such as Stackdriver / Google Cloud Operations / GCP Log Explorer).

Usage:

	// OverwriteHandler
	overwriter := slogdedup.NewOverwriteHandler(slog.NewJSONHandler(os.Stdout, nil), nil)
	slog.SetDefault(slog.New(overwriter))

	// {
	//   "time": "2024-03-21T09:33:25Z",
	//   "level": "INFO",
	//   "msg": "this is the dedup overwrite handler",
	//   "duplicated": "two"
	// }
	slog.Info("this is the dedup overwrite handler",
		slog.String("duplicated", "zero"),
		slog.String("duplicated", "one"),
		slog.String("duplicated", "two"),
	)

	// IgnoreHandler
	ignorer := slogdedup.NewIgnoreHandler(slog.NewJSONHandler(os.Stdout, nil), nil)
	slog.SetDefault(slog.New(ignorer))

	// {
	//   "time": "2024-03-21T09:33:25Z",
	//   "level": "INFO",
	//   "msg": "this is the dedup ignore handler",
	//   "duplicated": "zero"
	// }
	slog.Info("this is the dedup ignore handler",
		slog.String("duplicated", "zero"),
		slog.String("duplicated", "one"),
		slog.String("duplicated", "two"),
	)

	// IncrementHandler
	incrementer := slogdedup.NewIncrementHandler(slog.NewJSONHandler(os.Stdout, nil), nil)
	slog.SetDefault(slog.New(incrementer))

	// {
	//   "time": "2024-03-21T09:33:25Z",
	//   "level": "INFO",
	//   "msg": "this is the dedup incrementer handler",
	//   "duplicated": "zero",
	//   "duplicated#01": "one",
	//   "duplicated#02": "two"
	// }
	slog.Info("this is the dedup incrementer handler",
		slog.String("duplicated", "zero"),
		slog.String("duplicated", "one"),
		slog.String("duplicated", "two"),
	)

	// AppendHandler
	appender := slogdedup.NewAppendHandler(slog.NewJSONHandler(os.Stdout, nil), nil)
	slog.SetDefault(slog.New(appender))

	// {
	//   "time": "2024-03-21T09:33:25Z",
	//   "level": "INFO",
	//   "msg": "this is the dedup appender handler",
	//   "duplicated": [
	//     "zero",
	//     "one",
	//     "two"
	//   ]
	// }
	slog.Info("this is the dedup appender handler",
		slog.String("duplicated", "zero"),
		slog.String("duplicated", "one"),
		slog.String("duplicated", "two"),
	)


	logger := slog.New(slogdedup.NewOverwriteHandler(
		slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			AddSource:   true,
			ReplaceAttr: slogdedup.ReplaceAttrStackdriver(), // Needed for builtin's
		}),
		&slogdedup.OverwriteHandlerOptions{ResolveKey: slogdedup.ResolveKeyStackdriver()}, // Needed for everything else, and deduplication
	))

	// {
	//   "time": "2024-03-21T09:59:19.652284-06:00",
	//   "severity": "WARNING",
	//   "logging.googleapis.com/sourceLocation": {
	//     "function": "main.main",
	//     "file": "/go/src/github.com/veqryn/slog-dedup/cmd/replacers/cmd.go",
	//     "line": "19"
	//   },
	//   "message": "this is the main message",
	//   "duplicated": "one"
	// }
	logger.Warn("this is the main message", slog.String("duplicated", "zero"), slog.String("duplicated", "one"))
*/
package slogdedup
