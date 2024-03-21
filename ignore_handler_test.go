package slogdedup

import (
	"log/slog"
	"strings"
	"testing"
)

/*
	{
		"time": "2023-09-29T13:00:59Z",
		"level": "INFO",
		"msg": "main message",
		"arg1": "with1arg1",
		"arg2": "with1arg2",
		"arg3": "with1arg3",
		"arg4": "with2arg4",
		"group1": "with2group1",
		"level#01": "with2level",
		"msg#01": "prexisting01",
		"msg#01a": "seekbug01a",
		"msg#02": "seekbug02",
		"source#01": "with1source",
		"time#01": "with1time",
		"typed": "overwritten",
		"with1": "arg0",
		"with2": "arg0"
	}
*/
func TestIgnoreHandler(t *testing.T) {
	t.Parallel()

	tester := &testHandler{}
	h := NewIgnoreHandler(tester, nil)

	logComplex(t, h)

	jBytes, err := tester.MarshalJSON()
	if err != nil {
		t.Errorf("Unable to marshal json: %v", err)
	}
	jStr := strings.TrimSpace(string(jBytes))

	expected := `{"time":"2023-09-29T13:00:59Z","level":"INFO","msg":"main message","arg1":"with1arg1","arg2":"with1arg2","arg3":"with1arg3","arg4":"with2arg4","group1":"with2group1","level#01":"with2level","msg#01":"prexisting01","msg#01a":"seekbug01a","msg#02":"seekbug02","source#01":"with1source","time#01":"with1time","typed":"overwritten","with1":"arg0","with2":"arg0"}`
	if jStr != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected, jStr)
	}

	// Uncomment to see the results
	// t.Error(jStr)
	// t.Error(tester.String())

	checkRecordForDuplicates(t, tester.Record)
}

func TestIgnoreHandler_ResolveBuiltinKeyConflict(t *testing.T) {
	t.Parallel()

	tester := &testHandler{}
	h := NewIgnoreMiddleware(&IgnoreHandlerOptions{
		ResolveKey: func(groups []string, key string, _ int) (string, bool) {
			if len(groups) > 0 {
				return key, true
			}
			if key == "time" {
				return "", false
			} else {
				return "arg-" + key, true
			}
		},
	})(tester)

	slog.New(h).Info("main message", "time", "hello", "foo", "bar")

	jBytes, err := tester.MarshalJSON()
	if err != nil {
		t.Errorf("Unable to marshal json: %v", err)
	}
	jStr := strings.TrimSpace(string(jBytes))

	expected := `{"time":"2023-09-29T13:00:59Z","level":"INFO","msg":"main message","arg-foo":"bar"}`
	if jStr != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected, jStr)
	}

	// Uncomment to see the results
	// t.Error(jStr)
	// t.Error(tester.String())

	checkRecordForDuplicates(t, tester.Record)
}
