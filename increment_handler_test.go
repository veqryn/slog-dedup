package slogdedup

import (
	"log/slog"
	"strconv"
	"strings"
	"testing"
)

/*
	{
		"time": "2023-09-29T13:00:59Z",
		"level": "WARN",
		"msg": "main message",
		"arg1": "with1arg1",
		"arg1#01": "with2arg1",
		"arg2": "with1arg2",
		"arg3": "with1arg3",
		"arg3#01": "with2arg3",
		"arg4": "with2arg4",
		"group1": "with2group1",
		"group1#01": {
			"arg1": "group1with3arg1",
			"arg1#01": "group1with4arg1",
			"arg1#02": "main1arg1",
			"arg2": "group1with3arg2",
			"arg3": "group1with3arg3",
			"arg3#01": "group1with4arg3",
			"arg4": "group1with4arg4",
			"arg5": "with4inlinedGroupArg5",
			"arg6": "main1arg6",
			"level": "with4overwritten",
			"level#01": "main1overwritten",
			"level#02": "main1level",
			"main1": "arg0",
			"main1group3": {
				"group3": "group3overwritten",
				"group3#01": "group3arg0"
			},
			"msg": "with4msg",
			"overwrittenGroup": {
				"arg": "arg"
			},
			"overwrittenGroup#01": "with4overwrittenGroup",
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
		"level#01": "with2level",
		"level#02": {
			"levelGroupKey": "levelGroupValue"
		},
		"level#03": {
			"inlinedLevelGroupKey": "inlinedLevelGroupValue"
		},
		"logging.googleapis.com/sourceLocation": "sourceLocationArg",
		"message": "messageArg",
		"message#01": "message#01Arg",
		"msg#01": "prexisting01",
		"msg#01a": "seekbug01a",
		"msg#02": "seekbug02",
		"msg#03": "with2msg",
		"msg#04": "with2msg2",
		"severity": "severityArg",
		"source#01": "with1source",
		"sourceLoc": "sourceLocArg",
		"time#01": "with1time",
		"timestamp": "timestampArg",
		"timestampRenamed": "timestampRenamedArg",
		"typed": "overwritten",
		"typed#01": 3,
		"typed#02": true,
		"with1": "arg0",
		"with2": "arg0"
	}
*/
func TestIncrementHandler(t *testing.T) {
	t.Parallel()

	tester := &testHandler{}
	h := NewIncrementHandler(tester, nil)

	logComplex(t, h)

	jBytes, err := tester.MarshalJSON()
	if err != nil {
		t.Errorf("Unable to marshal json: %v", err)
	}
	jStr := strings.TrimSpace(string(jBytes))

	expected := `{"time":"2023-09-29T13:00:59Z","level":"WARN","msg":"main message","arg1":"with1arg1","arg1#01":"with2arg1","arg2":"with1arg2","arg3":"with1arg3","arg3#01":"with2arg3","arg4":"with2arg4","group1":"with2group1","group1#01":{"arg1":"group1with3arg1","arg1#01":"group1with4arg1","arg1#02":"main1arg1","arg2":"group1with3arg2","arg3":"group1with3arg3","arg3#01":"group1with4arg3","arg4":"group1with4arg4","arg5":"with4inlinedGroupArg5","arg6":"main1arg6","level":"with4overwritten","level#01":"main1overwritten","level#02":"main1level","main1":"arg0","main1group3":{"group3":"group3overwritten","group3#01":"group3arg0"},"msg":"with4msg","overwrittenGroup":{"arg":"arg"},"overwrittenGroup#01":"with4overwrittenGroup","separateGroup2":{"arg1":"group2arg1","arg2":"group2arg2","group2":"group2arg0"},"source":"with3source","time":"with3time","with3":"arg0","with4":"arg0"},"level#01":"with2level","level#02":{"levelGroupKey":"levelGroupValue"},"level#03":{"inlinedLevelGroupKey":"inlinedLevelGroupValue"},"logging.googleapis.com/sourceLocation":"sourceLocationArg","message":"messageArg","message#01":"message#01Arg","msg#01":"prexisting01","msg#01a":"seekbug01a","msg#02":"seekbug02","msg#03":"with2msg","msg#04":"with2msg2","severity":"severityArg","source#01":"with1source","sourceLoc":"sourceLocArg","time#01":"with1time","timestamp":"timestampArg","timestampRenamed":"timestampRenamedArg","typed":"overwritten","typed#01":3,"typed#02":true,"with1":"arg0","with2":"arg0"}`
	if jStr != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected, jStr)
	}

	// Uncomment to see the results
	// t.Error(jStr)
	// t.Error(tester.String())

	checkRecordForDuplicates(t, tester.Record)
}

func TestIncrementHandler_DoesKeyConflict_IncrementKeyName(t *testing.T) {
	t.Parallel()

	tester := &testHandler{}
	h := NewIncrementMiddleware(&IncrementHandlerOptions{
		ResolveKey: func(groups []string, key string, index int) (string, bool) {
			if key == "foo" {
				return key + "@" + strconv.Itoa(index+1), true
			}
			if key == "hello" {
				return "", false
			}
			return key, true
		},
	})(tester)

	slog.New(h).Info("main message", "foo", "bar", "hello", "world")

	jBytes, err := tester.MarshalJSON()
	if err != nil {
		t.Errorf("Unable to marshal json: %v", err)
	}
	jStr := strings.TrimSpace(string(jBytes))

	expected := `{"time":"2023-09-29T13:00:59Z","level":"INFO","msg":"main message","foo@1":"bar"}`
	if jStr != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected, jStr)
	}

	// Uncomment to see the results
	// t.Error(jStr)
	// t.Error(tester.String())

	checkRecordForDuplicates(t, tester.Record)
}
