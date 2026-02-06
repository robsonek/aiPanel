package hosting

import (
	"context"
	"strings"
)

type fakeRunner struct {
	commands []string
	outputs  map[string]string
	errs     map[string]error
}

func (r *fakeRunner) Run(_ context.Context, name string, args ...string) (string, error) {
	cmd := strings.TrimSpace(name + " " + strings.Join(args, " "))
	r.commands = append(r.commands, cmd)
	if r.errs != nil {
		if err, ok := r.errs[cmd]; ok {
			out := ""
			if r.outputs != nil {
				out = r.outputs[cmd]
			}
			return out, err
		}
	}
	if r.outputs != nil {
		if out, ok := r.outputs[cmd]; ok {
			return out, nil
		}
	}
	return "", nil
}

func containsCommand(commands []string, want string) bool {
	for _, cmd := range commands {
		if cmd == want {
			return true
		}
	}
	return false
}
