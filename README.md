# slog-dedup
[![tag](https://img.shields.io/github/tag/veqryn/slog-dedup.svg)](https://github.com/veqryn/slog-dedup/releases)
![Go Version](https://img.shields.io/badge/Go-%3E%3D%201.21-%23007d9c)
[![GoDoc](https://godoc.org/github.com/veqryn/slog-dedup?status.svg)](https://pkg.go.dev/github.com/veqryn/slog-dedup)
![Build Status](https://github.com/veqryn/slog-dedup/actions/workflows/build_and_test.yml/badge.svg)
[![Go report](https://goreportcard.com/badge/github.com/veqryn/slog-dedup)](https://goreportcard.com/report/github.com/veqryn/slog-dedup)
[![Coverage](https://img.shields.io/codecov/c/github/veqryn/slog-dedup)](https://codecov.io/gh/veqryn/slog-dedup)
[![Contributors](https://img.shields.io/github/contributors/veqryn/slog-dedup)](https://github.com/veqryn/slog-dedup/graphs/contributors)
[![License](https://img.shields.io/github/license/veqryn/slog-dedup)](./LICENSE)

Golang structured logging (slog) deduplication for use with json logging (or any other format where duplicates are not appreciated).

The slog handlers in this module are "middleware" handlers. When creating them, you must pass in another handler, which will be called after this handler has finished handling a log record. Because of this, these handlers can be chained with other middlewares, and can be used with many different final handlers, whether from the stdlib or third-party, such as json, protobuf, text, or data sinks.

The main impetus behind this package is because most JSON tools do not like duplicate keys for their member properties/fields. Some of them will give errors or fail to parse the log line, and some may even crash.

Unfortunately the default behavior of the stdlib slog handlers is to allow duplicate keys:
```go
// This makes json tools unhappy    :(
slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
slog.Info("this is the stdlib json handler by itself",
    slog.String("duplicated", "zero"),
    slog.String("duplicated", "one"),
    slog.String("duplicated", "two"),
)
```
Outputs:
```json
{
    "time": "2023-10-03T01:45:00Z",
    "level": "INFO",
    "msg": "this is the stdlib json handler by itself",
    "duplicated": "zero",
    "duplicated": "one",
    "duplicated": "two"
}
```
With this in mind, this repo was created with several different ways of deduplicating the keys: overwriting, ignoring, appending, incrementing.
For example, incrementing:
```json
{
    "time": "2024-03-21T09:33:25Z",
    "level": "INFO",
    "msg": "this is the dedup incrementer handler",
    "duplicated": "zero",
    "duplicated#01": "one",
    "duplicated#02": "two"
}
```

Additionally, this library includes convenience methods for formatting output to
match what is expected for various log aggregation tools (such as Graylog), as
well as cloud providers (such as Stackdriver / Google Cloud Operations / GCP Log Explorer).

### Other Great SLOG Utilities
- [slogctx](https://github.com/veqryn/slog-context): Add attributes to context and have them automatically added to all log lines. Work with a logger stored in context.
- [slogotel](https://github.com/veqryn/slog-context/tree/main/otel): Automatically extract and add [OpenTelemetry](https://opentelemetry.io/) TraceID's to all log lines.
- [slogdedup](https://github.com/veqryn/slog-dedup): Middleware that deduplicates and sorts attributes. Particularly useful for JSON logging.
- [slogbugsnag](https://github.com/veqryn/slog-bugsnag): Middleware that pipes Errors to [Bugsnag](https://www.bugsnag.com/).
- [slogjson](https://github.com/veqryn/slog-json): Formatter that uses the [JSON v2](https://github.com/golang/go/discussions/63397) [library](https://github.com/go-json-experiment/json), with optional single-line pretty-printing.

## Install
`go get github.com/veqryn/slog-dedup`

```go
import (
	slogdedup "github.com/veqryn/slog-dedup"
)
```

## Usage
### Overwrite Older Duplicates Handler
```go
logger := slog.New(slogdedup.NewOverwriteHandler(slog.NewJSONHandler(os.Stdout, nil), nil))
logger.Info("this is the dedup overwrite handler",
    slog.String("duplicated", "zero"),
    slog.String("duplicated", "one"),
    slog.String("duplicated", "two"),
)
```
Outputs:
```json
{
    "time": "2024-03-21T09:33:25Z",
    "level": "INFO",
    "msg": "this is the dedup overwrite handler",
    "duplicated": "two"
}
```

### Ignore Newer Duplicates Handler
```go
logger := slog.New(slogdedup.NewIgnoreHandler(slog.NewJSONHandler(os.Stdout, nil), nil))
logger.Info("this is the dedup ignore handler",
    slog.String("duplicated", "zero"),
    slog.String("duplicated", "one"),
    slog.String("duplicated", "two"),
)
```
Outputs:
```json
{
    "time": "2024-03-21T09:33:25Z",
    "level": "INFO",
    "msg": "this is the dedup ignore handler",
    "duplicated": "zero"
}
```

### Increment Newer Duplicate Key Names Handler
```go
logger := slog.New(slogdedup.NewIncrementHandler(slog.NewJSONHandler(os.Stdout, nil), nil))
logger.Info("this is the dedup incrementer handler",
    slog.String("duplicated", "zero"),
    slog.String("duplicated", "one"),
    slog.String("duplicated", "two"),
)
```
Outputs:
```json
{
    "time": "2024-03-21T09:33:25Z",
    "level": "INFO",
    "msg": "this is the dedup incrementer handler",
    "duplicated": "zero",
    "duplicated#01": "one",
    "duplicated#02": "two"
}
```

### Append Duplicates Together in an Array Handler
```go
logger := slog.New(slogdedup.NewAppendHandler(slog.NewJSONHandler(os.Stdout, nil), nil))
logger.Info("this is the dedup appender handler",
    slog.String("duplicated", "zero"),
    slog.String("duplicated", "one"),
    slog.String("duplicated", "two"),
)
```
Outputs:
```json
{
    "time": "2024-03-21T09:33:25Z",
    "level": "INFO",
    "msg": "this is the dedup appender handler",
    "duplicated": [
      "zero",
      "one",
      "two"
    ]
}
```

### Using ResolveKey and ReplaceAttr
#### Stackdriver (GCP Cloud Operations / Google Log Explorer)
```go
logger := slog.New(slogdedup.NewOverwriteHandler(
	slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		AddSource:   true,
		ReplaceAttr: slogdedup.ReplaceAttrStackdriver(nil), // Needed for builtin's
	}),
	&slogdedup.OverwriteHandlerOptions{ResolveKey: slogdedup.ResolveKeyStackdriver(nil)}, // Needed for everything else, and deduplication
))
logger.Warn("this is the main message", slog.String("duplicated", "zero"), slog.String("duplicated", "one"))
```
Outputs:
```json
{
    "time": "2024-03-21T09:59:19.652284-06:00",
    "severity": "WARNING",
    "logging.googleapis.com/sourceLocation": {
        "function": "main.main",
        "file": "/go/src/github.com/veqryn/slog-dedup/cmd/replacers/cmd.go",
        "line": "19"
    },
    "message": "this is the main message",
    "duplicated": "one"
}
```

## Full Example Main File
### Basic Use of Each Handler
```go
package main

import (
	"log/slog"
	"os"

	slogdedup "github.com/veqryn/slog-dedup"
)

func main() {
	// OverwriteHandler
	overwriter := slogdedup.NewOverwriteHandler(slog.NewJSONHandler(os.Stdout, nil), nil)
	slog.SetDefault(slog.New(overwriter))

	/*
	  {
	    "time": "2024-03-21T09:33:25Z",
	    "level": "INFO",
	    "msg": "this is the dedup overwrite handler",
	    "duplicated": "two"
	  }
	*/
	slog.Info("this is the dedup overwrite handler",
		slog.String("duplicated", "zero"),
		slog.String("duplicated", "one"),
		slog.String("duplicated", "two"),
	)

	// IgnoreHandler
	ignorer := slogdedup.NewIgnoreHandler(slog.NewJSONHandler(os.Stdout, nil), nil)
	slog.SetDefault(slog.New(ignorer))

	/*
	  {
	    "time": "2024-03-21T09:33:25Z",
	    "level": "INFO",
	    "msg": "this is the dedup ignore handler",
	    "duplicated": "zero"
	  }
	*/
	slog.Info("this is the dedup ignore handler",
		slog.String("duplicated", "zero"),
		slog.String("duplicated", "one"),
		slog.String("duplicated", "two"),
	)

	// IncrementHandler
	incrementer := slogdedup.NewIncrementHandler(slog.NewJSONHandler(os.Stdout, nil), nil)
	slog.SetDefault(slog.New(incrementer))

	/*
	  {
	    "time": "2024-03-21T09:33:25Z",
	    "level": "INFO",
	    "msg": "this is the dedup incrementer handler",
	    "duplicated": "zero",
	    "duplicated#01": "one",
	    "duplicated#02": "two"
	  }
	*/
	slog.Info("this is the dedup incrementer handler",
		slog.String("duplicated", "zero"),
		slog.String("duplicated", "one"),
		slog.String("duplicated", "two"),
	)

	// AppendHandler
	appender := slogdedup.NewAppendHandler(slog.NewJSONHandler(os.Stdout, nil), nil)
	slog.SetDefault(slog.New(appender))

	/*
	  {
	    "time": "2024-03-21T09:33:25Z",
	    "level": "INFO",
	    "msg": "this is the dedup appender handler",
	    "duplicated": [
	      "zero",
	      "one",
	      "two"
	    ]
	  }
	*/
	slog.Info("this is the dedup appender handler",
		slog.String("duplicated", "zero"),
		slog.String("duplicated", "one"),
		slog.String("duplicated", "two"),
	)
}
```

### Using ResolveKey and ReplaceAttr
```go
package main

import (
	"log/slog"
	"os"

	slogdedup "github.com/veqryn/slog-dedup"
)

func main() {
	// Example sending logs of Stackdriver (GCP Cloud Operations / Google Log Explorer)
	// which then pipes the data to Graylog.

	// First, create a function to resolve/replace attribute keys before deduplication,
	// which will also ensure the builtin keys are unused by non-builtin attributes:
	resolveKey := slogdedup.JoinResolveKey(
		slogdedup.ResolveKeyStackdriver(nil),
		slogdedup.ResolveKeyGraylog(nil),
	)

	// Second, create a function to replace the builtin record attributes
	// (time, level, msg, source) with the appropriate keys and values for
	// Stackdriver and Graylog:
	replaceAttr := slogdedup.JoinReplaceAttr(
		slogdedup.ReplaceAttrStackdriver(nil),
		slogdedup.ReplaceAttrGraylog(nil),
	)

	// Next create the final handler (the sink), which is a json handler,
	// using the replaceAttr function we just made:
	jsonHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		AddSource:   true,
		Level:       slog.LevelDebug,
		ReplaceAttr: replaceAttr,
	})

	// Now create any one of the dedup middleware handlers, using the
	// resolveKey function we just made, then set it as the default logger:
	dedupHandler := slogdedup.NewOverwriteHandler(
		jsonHandler,
		&slogdedup.OverwriteHandlerOptions{
			KeyCompare: slogdedup.CaseSensitiveCmp, // this is the default
			ResolveKey: resolveKey,                 // use the key resolver for stackdriver/graylog
		},
	)
	slog.SetDefault(slog.New(dedupHandler))

	/*
		{
			"time": "2024-03-21T09:53:43Z",
			"severity": "WARNING",
			"logging.googleapis.com/sourceLocation": {
				"function": "main.main",
				"file": "/go/src/github.com/veqryn/slog-dedup/cmd/replacers/cmd.go",
				"line": "56"
			},
			"message": "this is the main message",
			"duplicated": "one"
		}
	*/
	slog.Warn("this is the main message", slog.String("duplicated", "zero"), slog.String("duplicated", "one"))
}
```

## Breaking Changes
### O.4.x -> 0.5.0
Resolvers and Replacers (such as `ResolveKeyStackdriver` and `ReplaceAttrStackdriver`)
now take an argument of `*ResolveReplaceOptions`, which can be nil for the default behavior.

### O.3.x -> 0.4.0
`ResolveBuiltinKeyConflict`,`DoesBuiltinKeyConflict`, and `IncrementKeyName` have
all been unified into a single function: `ResolveKey`.


### O.1.x -> 0.2.0
Package renamed from `dedup` to `slogdedup`.
To fix, change this:
```go
import "github.com/veqryn/slog-dedup"
var overwriter = dedup.NewOverwriteHandler(slog.NewJSONHandler(os.Stdout, nil), nil)
```
To this:
```go
import "github.com/veqryn/slog-dedup"
var overwriter = slogdedup.NewOverwriteHandler(slog.NewJSONHandler(os.Stdout, nil), nil)
```
Named imports are unaffected.

## Other Details
### slog-multi Middleware
This library has convenience methods that allow it to interoperate with [github.com/samber/slog-multi](https://github.com/samber/slog-multi),
in order to easily setup slog workflows such as pipelines, fanout, routing, failover, etc.
```go
slog.SetDefault(slog.New(slogmulti.
	Pipe(slogctx.NewMiddleware(&slogctx.HandlerOptions{})).
	Pipe(slogdedup.NewOverwriteMiddleware(&slogdedup.OverwriteHandlerOptions{})).
	Handler(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{})),
))
```

### Overwrite Handler
Using an overwrite handler allows a slightly different style of logging that is less verbose. As an application moves deeper into domain functions, it is common that additional details or knowledge is uncovered. By overwriting keys with better and more explanatory values as you go, the final log lines are often easier to read and more informative.

### WithAttrs, WithGroup, and slog.Group()
These handlers will correctly deal with sub-loggers, whether using `WithAttrs()` or `WithGroup()`. It will even handle groups injected as attributes using `slog.Group()`. Due to the lack of a `slog.Slice` type/kind, the `AppendHandler` has a special case where groups that are inside of slices/arrays are turned into a `map[string]any{}` slog attribute before being passed to the final handler.

### The Built-In Fields (time, level, msg, source)
Because this handler is a middleware, it must pass a `slog.Record` to the final handler. The built-in attributes for time, level, msg, and source are treated separately, and have their own fields on the `slog.Record` struct. It would therefore be impossible to deduplicate these, if we didn't handle these as a special case. The increment handler considers that these four keys are always taken at the root level, and any attributes using those keys will start with the #01 increment on their key name. The other handlers can be customized using their options struct to either increment the name (default), drop old attributes using those keys (overwrite with the final slog.Record builtins), or allow the duplicates for the builtin keys. You can also customize this behavior by passing your own functions to the options struct (same for log handlers that use different keys for the built-in fields).
