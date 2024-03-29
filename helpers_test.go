package slogdedup

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"
)

/*
What the log output would look like if duplicates were allowed, and sorted for easier reading:

	{
		"arg1": "with1arg1",
		"arg1": "with2arg1",
		"arg2": "with1arg2",
		"arg3": "with1arg3",
		"arg3": "with2arg3",
		"arg4": "with2arg4",
		"group1": "with2group1",
		"group1": {
			"arg1": "group1with3arg1",
			"arg1": "group1with4arg1",
			"arg1": "main1arg1",
			"arg2": "group1with3arg2",
			"arg3": "group1with3arg3",
			"arg3": "group1with4arg3",
			"arg4": "group1with4arg4",
			"arg5": "with4inlinedGroupArg5",
			"arg6": "main1arg6",
			"level": "with4overwritten",
			"level": "main1overwritten",
			"level": "main1level",
			"main1": "arg0",
			"main1group3": {
				"group3": "group3overwritten",
				"group3": "group3arg0"
			},
			"msg": "with4msg",
			"overwrittenGroup": {
				"arg": "arg"
			},
			"overwrittenGroup": "with4overwrittenGroup",
			"separateGroup2": {
				"arg1": "group2arg1",
				"arg2": "group2arg2",
				"group2": "group2arg0"
			},
			"source": "with3source",
			"time": "with3time",
			"with3": "arg0",
			"with4": "arg0"
		},
		"level": "WARN",
		"level": "with2level",
		"level": {
			"levelGroupKey": "levelGroupValue"
		},
		"level": {
			"inlinedLevelGroupKey": "inlinedLevelGroupValue"
		},
		"logging.googleapis.com/sourceLocation": "sourceLocationArg",
		"message": "messageArg",
		"message#01": "message#01Arg",
		"msg": "main message",
		"msg": "with2msg",
		"msg": "with2msg2",
		"msg#01": "prexisting01",
		"msg#01a": "seekbug01a",
		"msg#02": "seekbug02",
		"severity": "severityArg",
		"source": "with1source",
		"sourceLoc": "sourceLocArg",
		"time": "2024-03-29T16:18:25.924174-06:00",
		"time": "with1time",
		"timestamp": "timestampArg",
		"timestampRenamed": "timestampRenamedArg",
		"typed": "overwritten",
		"typed": 3,
		"typed": true,
		"with1": "arg0",
		"with2": "arg0"
	}
*/
func logComplex(t *testing.T, handler slog.Handler) {
	t.Helper()

	log := slog.New(handler)

	log = log.With("with1", "arg0", "arg1", "with1arg1", "arg2", "with1arg2", "arg3", "with1arg3", slog.SourceKey, "with1source", slog.TimeKey, "with1time", slog.Group("emptyGroup"), "typed", "overwritten", slog.Int("typed", 3))
	log = log.With("with2", "arg0", "arg1", "with2arg1", "arg3", "with2arg3", "arg4", "with2arg4", "msg#01", "prexisting01", "msg#01a", "seekbug01a", "msg#02", "seekbug02", slog.MessageKey, "with2msg", slog.MessageKey, "with2msg2", slog.LevelKey, "with2level", "group1", "with2group1", slog.Bool("typed", true))
	log = log.With("timestamp", "timestampArg", "timestampRenamed", "timestampRenamedArg", "severity", "severityArg", "message", "messageArg", "message#01", "message#01Arg", "sourceLoc", "sourceLocArg", "logging.googleapis.com/sourceLocation", "sourceLocationArg")
	log = log.With(slog.Group(slog.LevelKey, "levelGroupKey", "levelGroupValue"), slog.Group("", slog.Group(slog.LevelKey, "inlinedLevelGroupKey", "inlinedLevelGroupValue")))
	log = log.WithGroup("group1").With(slog.Attr{})
	log = log.With("with3", "arg0", "arg1", "group1with3arg1", "arg2", "group1with3arg2", "arg3", "group1with3arg3", slog.Group("overwrittenGroup", "arg", "arg"), slog.Group("separateGroup2", "group2", "group2arg0", "arg1", "group2arg1", "arg2", "group2arg2"), slog.SourceKey, "with3source", slog.TimeKey, "with3time")
	log = log.WithGroup("").WithGroup("")
	log = log.With("with4", "arg0", "arg1", "group1with4arg1", "arg3", "group1with4arg3", "arg4", "group1with4arg4", slog.Group("", "arg5", "with4inlinedGroupArg5"), slog.String("overwrittenGroup", "with4overwrittenGroup"), slog.MessageKey, "with4msg", slog.LevelKey, "with4overwritten")
	log.Warn("main message", "main1", "arg0", "arg1", "main1arg1", "arg6", "main1arg6", slog.LevelKey, "main1overwritten", slog.LevelKey, "main1level", slog.Group("main1group3", "group3", "group3overwritten", "group3", "group3arg0"))
}

func TestSlogJsonHandler(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	h := slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})

	logComplex(t, h)

	pretty := &bytes.Buffer{}
	err := json.Indent(pretty, buf.Bytes(), "", "    ")
	if err != nil {
		t.Fatal(err)
	}
	t.Log(pretty.String())
}

type testHandler struct {
	Ctx    context.Context
	Record slog.Record
}

func (h *testHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h *testHandler) Handle(ctx context.Context, r slog.Record) error {
	h.Ctx = ctx
	h.Record = r
	h.Record.Time = time.Date(2023, 9, 29, 13, 0, 59, 0, time.UTC)
	return nil
}

func (h *testHandler) WithGroup(string) slog.Handler {
	panic("shouldn't be called")
}

func (h *testHandler) WithAttrs([]slog.Attr) slog.Handler {
	panic("shouldn't be called")
}

func (h *testHandler) String() string {
	buf := &bytes.Buffer{}
	err := slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}).Handle(context.Background(), h.Record)
	if err != nil {
		panic(err)
	}
	return buf.String()
}

func (h *testHandler) MarshalJSON() ([]byte, error) {
	buf := &bytes.Buffer{}
	err := h.MarshalWith(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (h *testHandler) MarshalWith(handler slog.Handler) error {
	return handler.Handle(context.Background(), h.Record)
}

func checkRecordForDuplicates(t *testing.T, r slog.Record) {
	t.Helper()

	attrs := make([]slog.Attr, 0, r.NumAttrs()+4)
	attrs = append(attrs,
		slog.Time(slog.TimeKey, r.Time),
		slog.Int(slog.LevelKey, int(r.Level)),
		slog.String(slog.MessageKey, r.Message),
		slog.String(slog.SourceKey, "SOURCE"),
	)
	r.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, a)
		return true
	})

	checkForDuplicates(t, attrs)
}

func checkForDuplicates(t *testing.T, attrs []slog.Attr) {
	t.Helper()

	seen := make(map[string]struct{}, len(attrs))
	for _, a := range attrs {
		if _, ok := seen[a.Key]; ok {
			t.Errorf("Duplicate key found: %v", a)
		}
		if a.Equal(slog.Attr{}) {
			t.Errorf("Empty attributes are not allowed: %v", a)
		}
		seen[a.Key] = struct{}{}

		// Dive into any Groups:
		if a.Value.Kind() == slog.KindGroup {
			if a.Key == "" {
				t.Errorf("Groups with empty names should be inlined: %v", a)
			}
			checkForDuplicates(t, a.Value.Group())
		}
	}
}
