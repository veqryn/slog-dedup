package slogdedup_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"testing/slogtest"

	slogdedup "github.com/veqryn/slog-dedup"
)

func TestSlogtest(t *testing.T) {
	for _, test := range []struct {
		name  string
		new   func(io.Writer) slog.Handler
		parse func([]byte) (map[string]any, error)
	}{
		{"OverwriteHandler", func(w io.Writer) slog.Handler { return slogdedup.NewOverwriteHandler(slog.NewJSONHandler(w, nil), nil) }, parseJSON},
		{"IgnoreHandler", func(w io.Writer) slog.Handler { return slogdedup.NewIgnoreHandler(slog.NewJSONHandler(w, nil), nil) }, parseJSON},
		{"IncrementHandler", func(w io.Writer) slog.Handler { return slogdedup.NewIncrementHandler(slog.NewJSONHandler(w, nil), nil) }, parseJSON},
		{"AppendHandler", func(w io.Writer) slog.Handler { return slogdedup.NewAppendHandler(slog.NewJSONHandler(w, nil), nil) }, parseJSON},
	} {
		t.Run(test.name, func(t *testing.T) {
			var buf bytes.Buffer
			h := test.new(&buf)
			results := func() []map[string]any {
				ms, err := parseLines(buf.Bytes(), test.parse)
				if err != nil {
					t.Fatal(err)
				}
				return ms
			}
			if err := slogtest.TestHandler(h, results); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func parseLines(src []byte, parse func([]byte) (map[string]any, error)) ([]map[string]any, error) {
	var records []map[string]any
	for _, line := range bytes.Split(src, []byte{'\n'}) {
		if len(line) == 0 {
			continue
		}
		m, err := parse(line)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", string(line), err)
		}
		records = append(records, m)
	}
	return records, nil
}

func parseJSON(bs []byte) (map[string]any, error) {
	var m map[string]any
	if err := json.Unmarshal(bs, &m); err != nil {
		return nil, err
	}
	return m, nil
}
