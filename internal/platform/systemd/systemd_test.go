package systemd

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestIsIgnorablePipeReadErr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: true},
		{name: "file already closed", err: errors.New("read |0: file already closed"), want: true},
		{name: "use of closed file", err: errors.New("read from file: use of closed file"), want: true},
		{name: "other", err: errors.New("permission denied"), want: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isIgnorablePipeReadErr(tt.err); got != tt.want {
				t.Fatalf("isIgnorablePipeReadErr(%v)=%v want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestExecRunnerRunLive_ShellCommand(t *testing.T) {
	t.Parallel()

	r := ExecRunner{}
	out, err := r.RunLive(context.Background(), "sh", []string{"-c", "echo 33"}, nil)
	if err != nil {
		t.Fatalf("RunLive returned error: %v", err)
	}
	if strings.TrimSpace(out) != "33" {
		t.Fatalf("unexpected output: %q", out)
	}
}
