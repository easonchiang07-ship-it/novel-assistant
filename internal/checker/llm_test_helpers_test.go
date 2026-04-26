package checker

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
)

type fixedStreamer struct{ response string }

func (f *fixedStreamer) Stream(_ context.Context, _, _ string, w io.Writer) error {
	_, err := io.WriteString(w, f.response)
	return err
}

// captureStreamer 記錄每次呼叫的 system 與 prompt，並回傳固定 response。
type captureStreamer struct {
	response string
	calls    []capturedCall
}

type capturedCall struct {
	system string
	prompt string
}

func (c *captureStreamer) Stream(_ context.Context, system, prompt string, w io.Writer) error {
	c.calls = append(c.calls, capturedCall{system: system, prompt: prompt})
	_, err := io.WriteString(w, c.response)
	return err
}

// sequentialStreamer 依呼叫順序回傳不同 response，超出範圍時回傳最後一個。
type sequentialStreamer struct {
	responses []string
	n         atomic.Int32
}

func (s *sequentialStreamer) Stream(_ context.Context, _, _ string, w io.Writer) error {
	idx := int(s.n.Add(1)) - 1
	if idx >= len(s.responses) {
		idx = len(s.responses) - 1
	}
	_, err := io.WriteString(w, s.responses[idx])
	return err
}

// errorOnNthStreamer 在第 failOn 次（1-based）呼叫時回傳 error，其餘回傳固定 response。
type errorOnNthStreamer struct {
	response string
	failOn   int
	n        atomic.Int32
}

func (e *errorOnNthStreamer) Stream(_ context.Context, _, _ string, w io.Writer) error {
	n := int(e.n.Add(1))
	if n == e.failOn {
		return errors.New("simulated streamer error")
	}
	_, err := io.WriteString(w, e.response)
	return err
}
