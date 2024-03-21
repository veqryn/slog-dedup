package slogdedup

import (
	"log/slog"
	"strings"
	"testing"
)

/*
	{
		"time": "2023-09-29T13:00:59Z",
		"level": "WARN",
		"msg": "main message",
		"arg1": [
			"with1arg1",
			"with2arg1"
		],
		"arg2": "with1arg2",
		"arg3": [
			"with1arg3",
			"with2arg3"
		],
		"arg4": "with2arg4",
		"group1": [
			"with2group1",
			{
				"arg1": [
					"group1with3arg1",
					"group1with4arg1",
					"main1arg1"
				],
				"arg2": "group1with3arg2",
				"arg3": [
					"group1with3arg3",
					"group1with4arg3"
				],
				"arg4": "group1with4arg4",
				"arg5": "with4inlinedGroupArg5",
				"arg6": "main1arg6",
				"level": [
					"with4overwritten",
					"main1overwritten",
					"main1level"
				],
				"main1": "arg0",
				"main1group3": {
					"group3": [
						"group3overwritten",
						"group3arg0"
					]
				},
				"msg": "with4msg",
				"overwrittenGroup": [
					{
						"arg": "arg"
					},
					"with4overwrittenGroup"
				],
				"separateGroup2": {
					"arg1": "group2arg1",
					"arg2": "group2arg2",
					"group2": "group2arg0"
				},
				"source": "with3source",
				"time": "with3time",
				"with3": "arg0",
				"with4": "arg0"
			}
		],
		"level#01": [
			"with2level",
			{
				"levelGroupKey": "levelGroupValue"
			},
			{
				"inlinedLevelGroupKey": "inlinedLevelGroupValue"
			}
		],
		"logging.googleapis.com/sourceLocation": "sourceLocationArg",
		"message": "messageArg",
		"message#01": "message#01Arg",
		"msg#01": [
			"prexisting01",
			"with2msg",
			"with2msg2"
		],
		"msg#01a": "seekbug01a",
		"msg#02": "seekbug02",
		"severity": "severityArg",
		"source#01": "with1source",
		"sourceLoc": "sourceLocArg",
		"time#01": "with1time",
		"timestamp": "timestampArg",
		"timestampRenamed": "timestampRenamedArg",
		"typed": [
			"overwritten",
			3,
			true
		],
		"with1": "arg0",
		"with2": "arg0"
	}
*/
func TestAppendHandler(t *testing.T) {
	t.Parallel()

	tester := &testHandler{}
	h := NewAppendHandler(tester, nil)

	logComplex(t, h)

	jBytes, err := tester.MarshalJSON()
	if err != nil {
		t.Errorf("Unable to marshal json: %v", err)
	}
	jStr := strings.TrimSpace(string(jBytes))

	expected := `{"time":"2023-09-29T13:00:59Z","level":"WARN","msg":"main message","arg1":["with1arg1","with2arg1"],"arg2":"with1arg2","arg3":["with1arg3","with2arg3"],"arg4":"with2arg4","group1":["with2group1",{"arg1":["group1with3arg1","group1with4arg1","main1arg1"],"arg2":"group1with3arg2","arg3":["group1with3arg3","group1with4arg3"],"arg4":"group1with4arg4","arg5":"with4inlinedGroupArg5","arg6":"main1arg6","level":["with4overwritten","main1overwritten","main1level"],"main1":"arg0","main1group3":{"group3":["group3overwritten","group3arg0"]},"msg":"with4msg","overwrittenGroup":[{"arg":"arg"},"with4overwrittenGroup"],"separateGroup2":{"arg1":"group2arg1","arg2":"group2arg2","group2":"group2arg0"},"source":"with3source","time":"with3time","with3":"arg0","with4":"arg0"}],"level#01":["with2level",{"levelGroupKey":"levelGroupValue"},{"inlinedLevelGroupKey":"inlinedLevelGroupValue"}],"logging.googleapis.com/sourceLocation":"sourceLocationArg","message":"messageArg","message#01":"message#01Arg","msg#01":["prexisting01","with2msg","with2msg2"],"msg#01a":"seekbug01a","msg#02":"seekbug02","severity":"severityArg","source#01":"with1source","sourceLoc":"sourceLocArg","time#01":"with1time","timestamp":"timestampArg","timestampRenamed":"timestampRenamedArg","typed":["overwritten",3,true],"with1":"arg0","with2":"arg0"}`
	if jStr != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected, jStr)
	}

	// Uncomment to see the results
	// t.Error(jStr)
	// t.Error(tester.String())

	checkRecordForDuplicates(t, tester.Record)
}

/*
	{
	  "time": "2023-09-29T13:00:59Z",
	  "level": "INFO",
	  "msg": "case insenstive, keep builtin conflict",
	  "arg1": ["val1","val2"],
	  "msg":"builtin-conflict"
	}
*/
func TestAppendHandler_CaseInsensitiveKeepIfBuiltinConflict(t *testing.T) {
	t.Parallel()

	tester := &testHandler{}
	h := NewAppendMiddleware(&AppendHandlerOptions{
		KeyCompare: CaseInsensitiveCmp,
		ResolveKey: KeepIfBuiltinKeyConflict,
	})(tester)

	log := slog.New(h)
	log.Info("case insenstive, keep builtin conflict", "arg1", "val1", "ARG1", "val2", slog.MessageKey, "builtin-conflict")

	jBytes, err := tester.MarshalJSON()
	if err != nil {
		t.Errorf("Unable to marshal json: %v", err)
	}
	jStr := strings.TrimSpace(string(jBytes))

	expected := `{"time":"2023-09-29T13:00:59Z","level":"INFO","msg":"case insenstive, keep builtin conflict","arg1":["val1","val2"],"msg":"builtin-conflict"}`
	if jStr != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected, jStr)
	}

	// Uncomment to see the results
	// t.Error(jStr)
	// t.Error(tester.String())
}
