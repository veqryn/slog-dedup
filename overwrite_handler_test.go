package dedup

import (
	"log/slog"
	"os"
	"testing"
)

func TestOverwriteHandler(t *testing.T) {
	t.Parallel()

	// base := slog.NewTextHandler(os.Stdout, nil)
	base := slog.NewJSONHandler(os.Stdout, nil)
	slog.New(base).Info("hello", "", "emptykeysvalue", "emptyvalueskey", "", "foo", "bar", `"quotedkey"`, `"quotedvalue"`)
	slog.New(NewOverwriteHandler(base, &OverwriteHandlerOptions{})).Info("hello", "", "emptykeysvalue", "emptyvalueskey", "", "foo", "bar", `"quotedkey"`, `"quotedvalue"`)

	oh := NewOverwriteHandler(base, &OverwriteHandlerOptions{})
	log := slog.New(oh)

	log = log.With("with1", "arg0", "arg1", "with1arg1", "arg2", "with1arg2", "arg3", "with1arg3", slog.SourceKey, "with1source")
	log = log.With("with2", "arg0", "arg1", "with2arg1", "arg3", "with2arg3", "arg4", "with2arg4")
	log = log.WithGroup("group1")
	log = log.With("with3", "arg0", "arg1", "group1with3arg1", "arg2", "group1with3arg2", "arg3", "group1with3arg3", slog.Group("separateGroup2", "group2", "group2arg0", "arg1", "group2arg1", "arg2", "group2arg2"))
	log = log.With("with4", "arg0", "arg1", "group1with4arg1", "arg3", "group1with4arg3", "arg4", "group1with4arg4")
	log.Info("main message", "main1", "arg0", "arg1", "main1arg1", "arg5", "main1arg5")
}
