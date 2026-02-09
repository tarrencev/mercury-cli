package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/tarrence/mercury-cli/internal/cligen"
	"github.com/tarrence/mercury-cli/internal/mercuryhttp"
	"github.com/tarrence/mercury-cli/internal/openapi"
	"github.com/tarrence/mercury-cli/internal/output"
	"github.com/tarrence/mercury-cli/internal/version"
)

type rootOptions struct {
	Token string
	Env   string
	Auth  string

	BaseURL string
	Timeout time.Duration

	Pretty   bool
	NoPretty bool
	Ndjson   bool

	Debug bool
	Trace bool

	Status  bool
	Headers bool

	RetryNonIdempotent bool
}

type appState struct {
	opts    rootOptions
	client  *mercuryhttp.Client
	printer *output.Printer
}

func (a *appState) initFromFlags(cmd *cobra.Command) error {
	if a.opts.Pretty && a.opts.NoPretty {
		return fmt.Errorf("cannot set both --pretty and --no-pretty")
	}

	a.printer = output.NewPrinter(cmd.OutOrStdout(), cmd.ErrOrStderr(), output.PrinterOptions{
		ForcePretty:  a.opts.Pretty,
		ForceCompact: a.opts.NoPretty,
		Ndjson:       a.opts.Ndjson,
		PrintStatus:  a.opts.Status,
		PrintHeaders: a.opts.Headers,
	})

	httpClient, err := mercuryhttp.NewClient(mercuryhttp.ClientOptions{
		Timeout:            a.opts.Timeout,
		Debug:              a.opts.Debug,
		Trace:              a.opts.Trace,
		RetryNonIdempotent: a.opts.RetryNonIdempotent,
		UserAgent:          version.UserAgent(),
		Out:                cmd.ErrOrStderr(),
	})
	if err != nil {
		return err
	}
	a.client = httpClient

	switch a.opts.Env {
	case "prod", "sandbox":
		// ok
	default:
		return fmt.Errorf("invalid --env %q (expected prod or sandbox)", a.opts.Env)
	}
	switch a.opts.Auth {
	case "bearer", "basic":
		// ok
	default:
		return fmt.Errorf("invalid --auth %q (expected bearer or basic)", a.opts.Auth)
	}
	return nil
}

func (a *appState) contextWithApp(ctx context.Context) context.Context {
	return context.WithValue(ctx, appKey{}, a)
}

type appKey struct{}

func appFrom(cmd *cobra.Command) (*appState, error) {
	v := cmd.Context().Value(appKey{})
	if v == nil {
		return nil, errors.New("internal error: app state missing from command context")
	}
	a, ok := v.(*appState)
	if !ok {
		return nil, errors.New("internal error: app state has wrong type")
	}
	return a, nil
}

func NewRootCmd() (*cobra.Command, error) {
	specDocs, err := openapi.LoadEmbeddedSpecs()
	if err != nil {
		return nil, err
	}

	app := &appState{
		opts: rootOptions{
			Env:     "prod",
			Auth:    "bearer",
			Timeout: 30 * time.Second,
		},
	}

	root := &cobra.Command{
		Use:   "mercury",
		Short: "Mercury Bank API CLI",
		Long: "Mercury Bank API CLI.\n\n" +
			"This CLI is generated from Mercury's published OpenAPI specs.\n\n" +
			"Authentication:\n" +
			"  export MERCURY_TOKEN=\"...\"\n" +
			"  mercury accounts get-accounts\n\n" +
			"Common usage:\n" +
			"  mercury <group> <operation> [path-args...] [--query/--header flags]\n\n" +
			"Examples:\n" +
			"  mercury accounts get-accounts --limit 100\n" +
			"  mercury accounts get-accounts --all\n" +
			"  mercury recipients create-recipient --data @recipient.json\n",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if err := app.initFromFlags(cmd); err != nil {
				return err
			}
			ctx := app.contextWithApp(cmd.Context())
			ctx = cligen.WithRuntime(ctx, &cligen.Runtime{
				Env:     app.opts.Env,
				BaseURL: app.opts.BaseURL,
				Token:   app.opts.Token,
				Auth:    app.opts.Auth,
				Client:  app.client,
				Printer: app.printer,
			})
			cmd.SetContext(ctx)
			return nil
		},
	}

	root.PersistentFlags().StringVar(&app.opts.Token, "token", "", "Mercury API token (or set MERCURY_TOKEN)")
	root.PersistentFlags().StringVar(&app.opts.Env, "env", app.opts.Env, "Environment: prod or sandbox")
	root.PersistentFlags().StringVar(&app.opts.Auth, "auth", app.opts.Auth, "Auth scheme for --token: bearer or basic")
	root.PersistentFlags().StringVar(&app.opts.BaseURL, "base-url", "", "Override server base URL (advanced)")
	root.PersistentFlags().DurationVar(&app.opts.Timeout, "timeout", app.opts.Timeout, "HTTP client timeout")

	root.PersistentFlags().BoolVar(&app.opts.Pretty, "pretty", false, "Force pretty-printed JSON output")
	root.PersistentFlags().BoolVar(&app.opts.NoPretty, "no-pretty", false, "Force compact (non-pretty) output")
	root.PersistentFlags().BoolVar(&app.opts.Ndjson, "ndjson", false, "Output newline-delimited JSON where applicable (primarily with --all)")

	root.PersistentFlags().BoolVar(&app.opts.Debug, "debug", false, "Log request/response metadata to stderr (redacts auth)")
	root.PersistentFlags().BoolVar(&app.opts.Trace, "trace", false, "Log full request/response bodies to stderr (redacts auth headers)")
	root.PersistentFlags().BoolVar(&app.opts.Status, "status", false, "Print HTTP status code to stderr")
	root.PersistentFlags().BoolVar(&app.opts.Headers, "headers", false, "Print response headers to stderr (redacts auth-related headers)")
	root.PersistentFlags().BoolVar(&app.opts.RetryNonIdempotent, "retry-non-idempotent", false, "Allow retries for non-idempotent requests on 429/5xx")

	root.SetVersionTemplate("{{.Version}}\n")
	root.Version = version.Version()

	// Built-ins
	root.AddCommand(newSpecCmd(specDocs))
	root.AddCommand(newVersionCmd())

	// Generated API commands
	if err := cligen.AddOpenAPICommands(root, specDocs); err != nil {
		return nil, err
	}

	// Env default from MERCURY_ENV, token default from MERCURY_TOKEN
	if v := os.Getenv("MERCURY_ENV"); v != "" {
		_ = root.PersistentFlags().Set("env", v)
	}
	if v := os.Getenv("MERCURY_TOKEN"); v != "" {
		_ = root.PersistentFlags().Set("token", v)
	}

	return root, nil
}
