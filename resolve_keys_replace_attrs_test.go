package slogdedup

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// Replaces the source file location so that tests pass on other people's systems.
func sourceReplacer(groups []string, a slog.Attr) slog.Attr {
	if len(groups) == 0 && a.Key == slog.SourceKey {
		src := a.Value.Any().(*slog.Source)
		src.File = "github.com/veqryn/slog-dedup/helpers_test.go"
		// src.Function = "github.com/veqryn/slog-dedup.logComplex"
		src.Line = 85
	}
	return a
}

func TestResolveKeyReplaceAttrStackdriver(t *testing.T) {
	t.Parallel()

	tester := &testHandler{}
	handler := NewIncrementHandler(tester, &IncrementHandlerOptions{ResolveKey: ResolveKeyStackdriver(&ResolveReplaceOptions{OverwriteSummary: true})})

	log := slog.New(handler)
	log.Info("Hello World", "Foo", "Bar")

	buf := &bytes.Buffer{}
	err := tester.MarshalWith(slog.NewJSONHandler(buf, &slog.HandlerOptions{AddSource: true, ReplaceAttr: JoinReplaceAttr(sourceReplacer, ReplaceAttrStackdriver(&ResolveReplaceOptions{OverwriteSummary: true}))}))
	if err != nil {
		t.Fatal(err)
	}

	// This is what it would be without using the replacers:
	// {"time":"2023-09-29T13:00:59Z","level":"INFO","source":{"function":"github.com/veqryn/slog-dedup.TestResolveKeyReplaceAttrStackdriver","file":"/Users/foobar/go/src/github.com/veqryn/slog-dedup/resolve_keys_replace_attrs_test.go","line":17},"msg":"Hello World","Foo":"Bar"}

	// And this is with the replacers:
	expected := `{"time":"2023-09-29T13:00:59Z","severity":"INFO","logging.googleapis.com/sourceLocation":{"function":"github.com/veqryn/slog-dedup.TestResolveKeyReplaceAttrStackdriver","file":"github.com/veqryn/slog-dedup/helpers_test.go","line":"85"},"message":"Hello World","Foo":"Bar"}`

	jStr := strings.TrimSpace(buf.String())
	if jStr != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s\n", expected, jStr)
	}

	checkRecordForDuplicates(t, tester.Record)
}

func TestResolveKeyReplaceAttrGraylog(t *testing.T) {
	t.Parallel()

	tester := &testHandler{}
	handler := NewIncrementHandler(tester, &IncrementHandlerOptions{ResolveKey: ResolveKeyGraylog(&ResolveReplaceOptions{OverwriteSummary: true})})

	log := slog.New(handler)
	log.Info("Hello World", "Foo", "Bar")

	buf := &bytes.Buffer{}
	err := tester.MarshalWith(slog.NewJSONHandler(buf, &slog.HandlerOptions{AddSource: true, ReplaceAttr: JoinReplaceAttr(sourceReplacer, ReplaceAttrGraylog(&ResolveReplaceOptions{OverwriteSummary: true}))}))
	if err != nil {
		t.Fatal(err)
	}

	// This is what it would be without using the replacers:
	// {"time":"2023-09-29T13:00:59Z","level":"INFO","source":{"function":"github.com/veqryn/slog-dedup.TestResolveKeyReplaceAttrGraylog","file":"/Users/foobar/go/src/github.com/veqryn/slog-dedup/resolve_keys_replace_attrs_test.go","line":46},"msg":"Hello World","Foo":"Bar"}

	// And this is with the replacers:
	expected := `{"time":"2023-09-29T13:00:59Z","level":"INFO","sourceLoc":{"function":"github.com/veqryn/slog-dedup.TestResolveKeyReplaceAttrGraylog","file":"github.com/veqryn/slog-dedup/helpers_test.go","line":85},"message":"Hello World","Foo":"Bar"}`

	jStr := strings.TrimSpace(buf.String())
	if jStr != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s\n", expected, jStr)
	}

	checkRecordForDuplicates(t, tester.Record)
}

func TestResolveKeyReplaceAttrCloudwatch(t *testing.T) {
	t.Parallel()

	tester := &testHandler{}
	handler := NewIncrementHandler(tester, nil) // no cloudwatch key resolver needed yet

	log := slog.New(handler)
	log.Info("Hello World", "Foo", "Bar")

	buf := &bytes.Buffer{}
	err := tester.MarshalWith(slog.NewJSONHandler(buf, &slog.HandlerOptions{AddSource: false, ReplaceAttr: ReplaceAttrCloudwatch(nil)}))
	if err != nil {
		t.Fatal(err)
	}

	// This is what it would be without using the replacers:
	// {"time":"2023-09-29T13:00:59Z","level":"INFO","msg":"Hello World","Foo":"Bar"}

	// And this is with the replacers:
	expected := `{"time":"2023-09-29T13:00:59.000000000Z","level":"INFO","msg":"Hello World","Foo":"Bar"}`

	jStr := strings.TrimSpace(buf.String())
	if jStr != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s\n", expected, jStr)
	}

	checkRecordForDuplicates(t, tester.Record)
}

func TestResolveKeyReplaceAttr(t *testing.T) {
	t.Parallel()

	defaultResolvers := JoinResolveKey(
		ResolveKeyStackdriver(nil),
		ResolveKeyGraylog(nil),
	)

	overwriteResolvers := JoinResolveKey(
		ResolveKeyStackdriver(&ResolveReplaceOptions{OverwriteSummary: true}),
		ResolveKeyGraylog(&ResolveReplaceOptions{OverwriteSummary: true}),
	)

	defaultReplacers := JoinReplaceAttr(
		sourceReplacer,
		ReplaceAttrStackdriver(nil),
		ReplaceAttrGraylog(nil),
	)

	overwriteReplacers := JoinReplaceAttr(
		sourceReplacer,
		ReplaceAttrStackdriver(&ResolveReplaceOptions{OverwriteSummary: true}),
		ReplaceAttrGraylog(&ResolveReplaceOptions{OverwriteSummary: true}),
	)

	tester := &testHandler{}
	tests := []struct {
		name       string
		middleware slog.Handler
		replacers  func(groups []string, a slog.Attr) slog.Attr
		expected   string
	}{
		{
			name:       "overwrite handler default resolve-replace",
			middleware: NewOverwriteHandler(tester, &OverwriteHandlerOptions{ResolveKey: defaultResolvers}),
			replacers:  defaultReplacers,
			expected:   `{"time":"2023-09-29T13:00:59Z","severity":"WARNING","logging.googleapis.com/sourceLocation":{"function":"github.com/veqryn/slog-dedup.logComplex","file":"github.com/veqryn/slog-dedup/helpers_test.go","line":"85"},"msg":"main message","arg1":"with2arg1","arg2":"with1arg2","arg3":"with2arg3","arg4":"with2arg4","group1":{"arg1":"main1arg1","arg2":"group1with3arg2","arg3":"group1with4arg3","arg4":"group1with4arg4","arg5":"with4inlinedGroupArg5","arg6":"main1arg6","level":"main1level","main1":"arg0","main1group3":{"group3":"group3arg0"},"msg":"with4msg","overwrittenGroup":"with4overwrittenGroup","separateGroup2":{"arg1":"group2arg1","arg2":"group2arg2","group2":"group2arg0"},"source":"with3source","time":"with3time","with3":"arg0","with4":"arg0"},"logging.googleapis.com/sourceLocation#01":"sourceLocationArg","message#01":"message#01Arg","msg#01":"with2msg2","msg#01a":"seekbug01a","msg#02":"seekbug02","severity#01":{"inlinedLevelGroupKey":"inlinedLevelGroupValue"},"sourceLoc#01":"sourceLocArg","time#01":"with1time","timestampRenamed":"timestampRenamedArg","typed":true,"with1":"arg0","with2":"arg0"}`,
		},
		{
			name:       "overwrite handler overwrite resolve-replace",
			middleware: NewOverwriteHandler(tester, &OverwriteHandlerOptions{ResolveKey: overwriteResolvers}),
			replacers:  overwriteReplacers,
			expected:   `{"time":"2023-09-29T13:00:59Z","severity":"WARNING","logging.googleapis.com/sourceLocation":{"function":"github.com/veqryn/slog-dedup.logComplex","file":"github.com/veqryn/slog-dedup/helpers_test.go","line":"85"},"message":"main message","arg1":"with2arg1","arg2":"with1arg2","arg3":"with2arg3","arg4":"with2arg4","group1":{"arg1":"main1arg1","arg2":"group1with3arg2","arg3":"group1with4arg3","arg4":"group1with4arg4","arg5":"with4inlinedGroupArg5","arg6":"main1arg6","level":"main1level","main1":"arg0","main1group3":{"group3":"group3arg0"},"msg":"with4msg","overwrittenGroup":"with4overwrittenGroup","separateGroup2":{"arg1":"group2arg1","arg2":"group2arg2","group2":"group2arg0"},"source":"with3source","time":"with3time","with3":"arg0","with4":"arg0"},"logging.googleapis.com/sourceLocation#01":"sourceLocationArg","message#01":"message#01Arg","msg#01":"prexisting01","msg#01a":"seekbug01a","msg#02":"seekbug02","severity#01":{"inlinedLevelGroupKey":"inlinedLevelGroupValue"},"sourceLoc#01":"sourceLocArg","time#01":"with1time","timestampRenamed":"timestampRenamedArg","typed":true,"with1":"arg0","with2":"arg0"}`,
		},
		{
			name:       "ignore handler default resolve-replace",
			middleware: NewIgnoreHandler(tester, &IgnoreHandlerOptions{ResolveKey: defaultResolvers}),
			replacers:  defaultReplacers,
			expected:   `{"time":"2023-09-29T13:00:59Z","severity":"WARNING","logging.googleapis.com/sourceLocation":{"function":"github.com/veqryn/slog-dedup.logComplex","file":"github.com/veqryn/slog-dedup/helpers_test.go","line":"85"},"msg":"main message","arg1":"with1arg1","arg2":"with1arg2","arg3":"with1arg3","arg4":"with2arg4","group1":"with2group1","logging.googleapis.com/sourceLocation#01":"with1source","message#01":"messageArg","msg#01":"prexisting01","msg#01a":"seekbug01a","msg#02":"seekbug02","severity#01":"with2level","sourceLoc#01":"sourceLocArg","time#01":"with1time","timestampRenamed":"timestampArg","typed":"overwritten","with1":"arg0","with2":"arg0"}`,
		},
		{
			name:       "ignore handler overwrite resolve-replace",
			middleware: NewIgnoreHandler(tester, &IgnoreHandlerOptions{ResolveKey: overwriteResolvers}),
			replacers:  overwriteReplacers,
			expected:   `{"time":"2023-09-29T13:00:59Z","severity":"WARNING","logging.googleapis.com/sourceLocation":{"function":"github.com/veqryn/slog-dedup.logComplex","file":"github.com/veqryn/slog-dedup/helpers_test.go","line":"85"},"message":"main message","arg1":"with1arg1","arg2":"with1arg2","arg3":"with1arg3","arg4":"with2arg4","group1":"with2group1","logging.googleapis.com/sourceLocation#01":"with1source","message#01":"with2msg","msg#01":"prexisting01","msg#01a":"seekbug01a","msg#02":"seekbug02","severity#01":"with2level","sourceLoc#01":"sourceLocArg","time#01":"with1time","timestampRenamed":"timestampArg","typed":"overwritten","with1":"arg0","with2":"arg0"}`,
		},
		{
			name:       "append handler default resolve-replace",
			middleware: NewAppendHandler(tester, &AppendHandlerOptions{ResolveKey: defaultResolvers}),
			replacers:  defaultReplacers,
			expected:   `{"time":"2023-09-29T13:00:59Z","severity":"WARNING","logging.googleapis.com/sourceLocation":{"function":"github.com/veqryn/slog-dedup.logComplex","file":"github.com/veqryn/slog-dedup/helpers_test.go","line":"85"},"msg":"main message","arg1":["with1arg1","with2arg1"],"arg2":"with1arg2","arg3":["with1arg3","with2arg3"],"arg4":"with2arg4","group1":["with2group1",{"arg1":["group1with3arg1","group1with4arg1","main1arg1"],"arg2":"group1with3arg2","arg3":["group1with3arg3","group1with4arg3"],"arg4":"group1with4arg4","arg5":"with4inlinedGroupArg5","arg6":"main1arg6","level":["with4overwritten","main1overwritten","main1level"],"main1":"arg0","main1group3":{"group3":["group3overwritten","group3arg0"]},"msg":"with4msg","overwrittenGroup":[{"arg":"arg"},"with4overwrittenGroup"],"separateGroup2":{"arg1":"group2arg1","arg2":"group2arg2","group2":"group2arg0"},"source":"with3source","time":"with3time","with3":"arg0","with4":"arg0"}],"logging.googleapis.com/sourceLocation#01":["with1source","sourceLocationArg"],"message#01":["messageArg","message#01Arg"],"msg#01":["prexisting01","with2msg","with2msg2"],"msg#01a":"seekbug01a","msg#02":"seekbug02","severity#01":["with2level","severityArg",{"levelGroupKey":"levelGroupValue"},{"inlinedLevelGroupKey":"inlinedLevelGroupValue"}],"sourceLoc#01":"sourceLocArg","time#01":"with1time","timestampRenamed":["timestampArg","timestampRenamedArg"],"typed":["overwritten",3,true],"with1":"arg0","with2":"arg0"}`,
		},
		{
			name:       "append handler overwrite resolve-replace",
			middleware: NewAppendHandler(tester, &AppendHandlerOptions{ResolveKey: overwriteResolvers}),
			replacers:  overwriteReplacers,
			expected:   `{"time":"2023-09-29T13:00:59Z","severity":"WARNING","logging.googleapis.com/sourceLocation":{"function":"github.com/veqryn/slog-dedup.logComplex","file":"github.com/veqryn/slog-dedup/helpers_test.go","line":"85"},"message":"main message","arg1":["with1arg1","with2arg1"],"arg2":"with1arg2","arg3":["with1arg3","with2arg3"],"arg4":"with2arg4","group1":["with2group1",{"arg1":["group1with3arg1","group1with4arg1","main1arg1"],"arg2":"group1with3arg2","arg3":["group1with3arg3","group1with4arg3"],"arg4":"group1with4arg4","arg5":"with4inlinedGroupArg5","arg6":"main1arg6","level":["with4overwritten","main1overwritten","main1level"],"main1":"arg0","main1group3":{"group3":["group3overwritten","group3arg0"]},"msg":"with4msg","overwrittenGroup":[{"arg":"arg"},"with4overwrittenGroup"],"separateGroup2":{"arg1":"group2arg1","arg2":"group2arg2","group2":"group2arg0"},"source":"with3source","time":"with3time","with3":"arg0","with4":"arg0"}],"logging.googleapis.com/sourceLocation#01":["with1source","sourceLocationArg"],"message#01":["with2msg","with2msg2","messageArg","message#01Arg"],"msg#01":"prexisting01","msg#01a":"seekbug01a","msg#02":"seekbug02","severity#01":["with2level","severityArg",{"levelGroupKey":"levelGroupValue"},{"inlinedLevelGroupKey":"inlinedLevelGroupValue"}],"sourceLoc#01":"sourceLocArg","time#01":"with1time","timestampRenamed":["timestampArg","timestampRenamedArg"],"typed":["overwritten",3,true],"with1":"arg0","with2":"arg0"}`,
		},
		{
			name:       "increment handler default resolve-replace",
			middleware: NewIncrementHandler(tester, &IncrementHandlerOptions{ResolveKey: defaultResolvers}),
			replacers:  defaultReplacers,
			expected:   `{"time":"2023-09-29T13:00:59Z","severity":"WARNING","logging.googleapis.com/sourceLocation":{"function":"github.com/veqryn/slog-dedup.logComplex","file":"github.com/veqryn/slog-dedup/helpers_test.go","line":"85"},"msg":"main message","arg1":"with1arg1","arg1#01":"with2arg1","arg2":"with1arg2","arg3":"with1arg3","arg3#01":"with2arg3","arg4":"with2arg4","group1":"with2group1","group1#01":{"arg1":"group1with3arg1","arg1#01":"group1with4arg1","arg1#02":"main1arg1","arg2":"group1with3arg2","arg3":"group1with3arg3","arg3#01":"group1with4arg3","arg4":"group1with4arg4","arg5":"with4inlinedGroupArg5","arg6":"main1arg6","level":"with4overwritten","level#01":"main1overwritten","level#02":"main1level","main1":"arg0","main1group3":{"group3":"group3overwritten","group3#01":"group3arg0"},"msg":"with4msg","overwrittenGroup":{"arg":"arg"},"overwrittenGroup#01":"with4overwrittenGroup","separateGroup2":{"arg1":"group2arg1","arg2":"group2arg2","group2":"group2arg0"},"source":"with3source","time":"with3time","with3":"arg0","with4":"arg0"},"logging.googleapis.com/sourceLocation#01":"with1source","logging.googleapis.com/sourceLocation#02":"sourceLocationArg","message#01":"messageArg","message#01#01":"message#01Arg","msg#01":"prexisting01","msg#01a":"seekbug01a","msg#02":"seekbug02","msg#03":"with2msg","msg#04":"with2msg2","severity#01":"with2level","severity#02":"severityArg","severity#03":{"levelGroupKey":"levelGroupValue"},"severity#04":{"inlinedLevelGroupKey":"inlinedLevelGroupValue"},"sourceLoc#01":"sourceLocArg","time#01":"with1time","timestampRenamed":"timestampArg","timestampRenamed#01":"timestampRenamedArg","typed":"overwritten","typed#01":3,"typed#02":true,"with1":"arg0","with2":"arg0"}`,
		},
		{
			name:       "increment handler overwrite resolve-replace",
			middleware: NewIncrementHandler(tester, &IncrementHandlerOptions{ResolveKey: overwriteResolvers}),
			replacers:  overwriteReplacers,
			expected:   `{"time":"2023-09-29T13:00:59Z","severity":"WARNING","logging.googleapis.com/sourceLocation":{"function":"github.com/veqryn/slog-dedup.logComplex","file":"github.com/veqryn/slog-dedup/helpers_test.go","line":"85"},"message":"main message","arg1":"with1arg1","arg1#01":"with2arg1","arg2":"with1arg2","arg3":"with1arg3","arg3#01":"with2arg3","arg4":"with2arg4","group1":"with2group1","group1#01":{"arg1":"group1with3arg1","arg1#01":"group1with4arg1","arg1#02":"main1arg1","arg2":"group1with3arg2","arg3":"group1with3arg3","arg3#01":"group1with4arg3","arg4":"group1with4arg4","arg5":"with4inlinedGroupArg5","arg6":"main1arg6","level":"with4overwritten","level#01":"main1overwritten","level#02":"main1level","main1":"arg0","main1group3":{"group3":"group3overwritten","group3#01":"group3arg0"},"msg":"with4msg","overwrittenGroup":{"arg":"arg"},"overwrittenGroup#01":"with4overwrittenGroup","separateGroup2":{"arg1":"group2arg1","arg2":"group2arg2","group2":"group2arg0"},"source":"with3source","time":"with3time","with3":"arg0","with4":"arg0"},"logging.googleapis.com/sourceLocation#01":"with1source","logging.googleapis.com/sourceLocation#02":"sourceLocationArg","message#01":"with2msg","message#01#01":"message#01Arg","message#02":"with2msg2","message#03":"messageArg","msg#01":"prexisting01","msg#01a":"seekbug01a","msg#02":"seekbug02","severity#01":"with2level","severity#02":"severityArg","severity#03":{"levelGroupKey":"levelGroupValue"},"severity#04":{"inlinedLevelGroupKey":"inlinedLevelGroupValue"},"sourceLoc#01":"sourceLocArg","time#01":"with1time","timestampRenamed":"timestampArg","timestampRenamed#01":"timestampRenamedArg","typed":"overwritten","typed#01":3,"typed#02":true,"with1":"arg0","with2":"arg0"}`,
		},
	}

	for _, testCase := range tests {
		logComplex(t, testCase.middleware)

		buf := &bytes.Buffer{}
		err := tester.MarshalWith(slog.NewJSONHandler(buf, &slog.HandlerOptions{AddSource: true, Level: slog.LevelDebug, ReplaceAttr: testCase.replacers}))
		if err != nil {
			t.Errorf("Unable to marshal json: %v", err)
			continue
		}
		jStr := strings.TrimSpace(buf.String())

		if jStr != testCase.expected {
			t.Errorf("%s Expected:\n%s\nGot:\n%s", testCase.name, testCase.expected, jStr)
		}

		// Uncomment to see the results
		// t.Error(jStr)
		// t.Error(tester.String())

		checkRecordForDuplicates(t, tester.Record)
	}
}
