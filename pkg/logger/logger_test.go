package logger

import (
	"bytes"
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"
)

func TestStringHandlerClonesSerializeWrites(t *testing.T) {
	var output bytes.Buffer
	base := newStringHandler(&output, slog.LevelDebug)
	handlers := []slog.Handler{base, base.WithAttrs([]slog.Attr{slog.String("scope", "attrs")}), base.WithGroup("group")}

	const writes = 100
	var wait sync.WaitGroup
	for i := 0; i < writes; i++ {
		handler := handlers[i%len(handlers)]
		wait.Add(1)
		go func() {
			defer wait.Done()
			record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
			if err := handler.Handle(context.Background(), record); err != nil {
				t.Errorf("Handle() error = %v", err)
			}
		}()
	}
	wait.Wait()

	if lines := bytes.Count(output.Bytes(), []byte("\n")); lines != writes {
		t.Fatalf("written lines = %d, want %d", lines, writes)
	}
}
