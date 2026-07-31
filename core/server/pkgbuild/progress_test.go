package pkgbuild

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

type recordingSink struct {
	milestones []string
	details    []string
}

func (s *recordingSink) Progress(step string, percent int, message string) {
	s.milestones = append(s.milestones, fmt.Sprintf("[%d%%] %s: %s", percent, step, message))
}

func (s *recordingSink) Logf(format string, args ...any) {
	s.details = append(s.details, fmt.Sprintf(format, args...))
}

func TestTimeStep_BracketsSuccess(t *testing.T) {
	sink := &recordingSink{}
	if err := TimeStep(sink, "assemble", func() error { return nil }); err != nil {
		t.Fatalf("TimeStep: %v", err)
	}
	if len(sink.details) != 2 {
		t.Fatalf("expected START + done lines, got %v", sink.details)
	}
	if sink.details[0] != "assemble: started" {
		t.Fatalf("start line wrong: %q", sink.details[0])
	}
	if !strings.HasPrefix(sink.details[1], "assemble: done in ") {
		t.Fatalf("done line wrong: %q", sink.details[1])
	}
}

func TestTimeStep_ReturnsErrorUnchangedAndLogsFailure(t *testing.T) {
	sink := &recordingSink{}
	boom := errors.New("boom")
	if err := TimeStep(sink, "build", func() error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("error not returned unchanged: %v", err)
	}
	if len(sink.details) != 2 || !strings.Contains(sink.details[1], "build: FAILED after") {
		t.Fatalf("failure line missing: %v", sink.details)
	}
}

func TestTimeStep_TolerateNilSink(t *testing.T) {
	if err := TimeStep(nil, "step", func() error { return nil }); err != nil {
		t.Fatalf("nil sink should be tolerated: %v", err)
	}
}
