package slogdedup

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestResolveKeyReplaceAttr(t *testing.T) {
	t.Parallel()

	resolvers := JoinResolveKey(
		ResolveKeyStackdriver(),
		ResolveKeyGraylog(),
	)

	replacers := JoinReplaceAttr(
		func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) == 0 && a.Key == slog.SourceKey {
				src := a.Value.Any().(*slog.Source)
				src.File = "github.com/veqryn/slog-dedup/helpers_test.go"
				src.Function = "github.com/veqryn/slog-dedup.logComplex"
				src.Line = 85
			}
			return a
		},
		ReplaceAttrStackdriver(),
		ReplaceAttrGraylog(),
	)

	tester := &testHandler{}
	tests := []struct {
		hander   slog.Handler
		expected string
	}{
		{
			hander:   NewOverwriteHandler(tester, &OverwriteHandlerOptions{ResolveKey: resolvers}),
			expected: `{"time":"2023-09-29T13:00:59Z","severity":"WARNING","logging.googleapis.com/sourceLocation":{"function":"github.com/veqryn/slog-dedup.logComplex","file":"github.com/veqryn/slog-dedup/helpers_test.go","line":"85"},"message":"main message","arg1":"with2arg1","arg2":"with1arg2","arg3":"with2arg3","arg4":"with2arg4","group1":{"arg1":"main1arg1","arg2":"group1with3arg2","arg3":"group1with4arg3","arg4":"group1with4arg4","arg5":"with4inlinedGroupArg5","arg6":"main1arg6","level":"main1level","main1":"arg0","main1group3":{"group3":"group3arg0"},"msg":"with4msg","overwrittenGroup":"with4overwrittenGroup","separateGroup2":{"arg1":"group2arg1","arg2":"group2arg2","group2":"group2arg0"},"source":"with3source","time":"with3time","with3":"arg0","with4":"arg0"},"logging.googleapis.com/sourceLocation#01":"sourceLocationArg","message#01":"message#01Arg","msg#01":"prexisting01","msg#01a":"seekbug01a","msg#02":"seekbug02","severity#01":{"inlinedLevelGroupKey":"inlinedLevelGroupValue"},"sourceLoc#01":"sourceLocArg","time#01":"with1time","timestampRenamed":"timestampRenamedArg","typed":true,"with1":"arg0","with2":"arg0"}`,
		},
		{
			hander:   NewIgnoreHandler(tester, &IgnoreHandlerOptions{ResolveKey: resolvers}),
			expected: `{"time":"2023-09-29T13:00:59Z","severity":"WARNING","logging.googleapis.com/sourceLocation":{"function":"github.com/veqryn/slog-dedup.logComplex","file":"github.com/veqryn/slog-dedup/helpers_test.go","line":"85"},"message":"main message","arg1":"with1arg1","arg2":"with1arg2","arg3":"with1arg3","arg4":"with2arg4","group1":"with2group1","logging.googleapis.com/sourceLocation#01":"with1source","message#01":"with2msg","msg#01":"prexisting01","msg#01a":"seekbug01a","msg#02":"seekbug02","severity#01":"with2level","sourceLoc#01":"sourceLocArg","time#01":"with1time","timestampRenamed":"timestampArg","typed":"overwritten","with1":"arg0","with2":"arg0"}`,
		},
		{
			hander:   NewAppendHandler(tester, &AppendHandlerOptions{ResolveKey: resolvers}),
			expected: `{"time":"2023-09-29T13:00:59Z","severity":"WARNING","logging.googleapis.com/sourceLocation":{"function":"github.com/veqryn/slog-dedup.logComplex","file":"github.com/veqryn/slog-dedup/helpers_test.go","line":"85"},"message":"main message","arg1":["with1arg1","with2arg1"],"arg2":"with1arg2","arg3":["with1arg3","with2arg3"],"arg4":"with2arg4","group1":["with2group1",{"arg1":["group1with3arg1","group1with4arg1","main1arg1"],"arg2":"group1with3arg2","arg3":["group1with3arg3","group1with4arg3"],"arg4":"group1with4arg4","arg5":"with4inlinedGroupArg5","arg6":"main1arg6","level":["with4overwritten","main1overwritten","main1level"],"main1":"arg0","main1group3":{"group3":["group3overwritten","group3arg0"]},"msg":"with4msg","overwrittenGroup":[{"arg":"arg"},"with4overwrittenGroup"],"separateGroup2":{"arg1":"group2arg1","arg2":"group2arg2","group2":"group2arg0"},"source":"with3source","time":"with3time","with3":"arg0","with4":"arg0"}],"logging.googleapis.com/sourceLocation#01":["with1source","sourceLocationArg"],"message#01":["with2msg","with2msg2","messageArg","message#01Arg"],"msg#01":"prexisting01","msg#01a":"seekbug01a","msg#02":"seekbug02","severity#01":["with2level","severityArg",{"levelGroupKey":"levelGroupValue"},{"inlinedLevelGroupKey":"inlinedLevelGroupValue"}],"sourceLoc#01":"sourceLocArg","time#01":"with1time","timestampRenamed":["timestampArg","timestampRenamedArg"],"typed":["overwritten",3,true],"with1":"arg0","with2":"arg0"}`,
		},
		{
			hander:   NewIncrementHandler(tester, &IncrementHandlerOptions{ResolveKey: resolvers}),
			expected: `{"time":"2023-09-29T13:00:59Z","severity":"WARNING","logging.googleapis.com/sourceLocation":{"function":"github.com/veqryn/slog-dedup.logComplex","file":"github.com/veqryn/slog-dedup/helpers_test.go","line":"85"},"message":"main message","arg1":"with1arg1","arg1#01":"with2arg1","arg2":"with1arg2","arg3":"with1arg3","arg3#01":"with2arg3","arg4":"with2arg4","group1":"with2group1","group1#01":{"arg1":"group1with3arg1","arg1#01":"group1with4arg1","arg1#02":"main1arg1","arg2":"group1with3arg2","arg3":"group1with3arg3","arg3#01":"group1with4arg3","arg4":"group1with4arg4","arg5":"with4inlinedGroupArg5","arg6":"main1arg6","level":"with4overwritten","level#01":"main1overwritten","level#02":"main1level","main1":"arg0","main1group3":{"group3":"group3overwritten","group3#01":"group3arg0"},"msg":"with4msg","overwrittenGroup":{"arg":"arg"},"overwrittenGroup#01":"with4overwrittenGroup","separateGroup2":{"arg1":"group2arg1","arg2":"group2arg2","group2":"group2arg0"},"source":"with3source","time":"with3time","with3":"arg0","with4":"arg0"},"logging.googleapis.com/sourceLocation#01":"with1source","logging.googleapis.com/sourceLocation#02":"sourceLocationArg","message#01":"with2msg","message#01#01":"message#01Arg","message#02":"with2msg2","message#03":"messageArg","msg#01":"prexisting01","msg#01a":"seekbug01a","msg#02":"seekbug02","severity#01":"with2level","severity#02":"severityArg","severity#03":{"levelGroupKey":"levelGroupValue"},"severity#04":{"inlinedLevelGroupKey":"inlinedLevelGroupValue"},"sourceLoc#01":"sourceLocArg","time#01":"with1time","timestampRenamed":"timestampArg","timestampRenamed#01":"timestampRenamedArg","typed":"overwritten","typed#01":3,"typed#02":true,"with1":"arg0","with2":"arg0"}`,
		},
	}

	for _, testCase := range tests {
		logComplex(t, testCase.hander)

		buf := &bytes.Buffer{}
		err := tester.MarshalWith(slog.NewJSONHandler(buf, &slog.HandlerOptions{AddSource: true, Level: slog.LevelDebug, ReplaceAttr: replacers}))
		if err != nil {
			t.Errorf("Unable to marshal json: %v", err)
			continue
		}
		jStr := strings.TrimSpace(buf.String())

		if jStr != testCase.expected {
			t.Errorf("Expected:\n%s\nGot:\n%s", testCase.expected, jStr)
		}

		// Uncomment to see the results
		// t.Error(jStr)
		// t.Error(tester.String())

		checkRecordForDuplicates(t, tester.Record)
	}
}
