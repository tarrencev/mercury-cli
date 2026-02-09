package cligen

import (
	"context"
	"errors"

	"github.com/spf13/cobra"
	"github.com/tarrence/mercury-cli/internal/mercuryhttp"
	"github.com/tarrence/mercury-cli/internal/output"
)

type Runtime struct {
	Env     string // prod|sandbox
	BaseURL string

	Token string
	Auth  string // bearer|basic

	Client  *mercuryhttp.Client
	Printer *output.Printer
}

type runtimeKey struct{}

func WithRuntime(ctx context.Context, rt *Runtime) context.Context {
	return context.WithValue(ctx, runtimeKey{}, rt)
}

func RuntimeFrom(cmd *cobra.Command) (*Runtime, error) {
	v := cmd.Context().Value(runtimeKey{})
	if v == nil {
		return nil, errors.New("internal error: runtime missing from context")
	}
	rt, ok := v.(*Runtime)
	if !ok || rt == nil {
		return nil, errors.New("internal error: runtime has wrong type")
	}
	if rt.Client == nil {
		return nil, errors.New("internal error: HTTP client missing from runtime")
	}
	if rt.Printer == nil {
		return nil, errors.New("internal error: printer missing from runtime")
	}
	return rt, nil
}
