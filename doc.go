/*
Package slogdedup provides structured logging (slog) deduplication for use with json logging
(or any other format where duplicates are not appreciated).
The main impetus behind this package is because most JSON tools do not like duplicate keys for their member
properties/fields. Some of them will give errors or fail to parse the log line, and some may even crash.
Unfortunately the default behavior of the stdlib slog handlers is to allow duplicate keys.

Usage:

	// OverwriteHandler
	overwriter := slogdedup.NewOverwriteHandler(slog.NewJSONHandler(os.Stdout, nil), nil)
	slog.SetDefault(slog.New(overwriter))

	// {
	//   "time": "2023-10-03T01:30:00Z",
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
	//   "time": "2023-10-03T01:30:00Z",
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
	//   "time": "2023-10-03T01:30:00Z",
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
	//   "time": "2023-10-03T01:30:00Z",
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
*/
package slogdedup
