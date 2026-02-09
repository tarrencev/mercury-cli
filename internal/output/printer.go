package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"golang.org/x/term"
)

type PrinterOptions struct {
	ForcePretty  bool
	ForceCompact bool
	Ndjson       bool

	PrintStatus  bool
	PrintHeaders bool
}

type Printer struct {
	out io.Writer
	err io.Writer

	pretty bool

	ndjson bool

	printStatus  bool
	printHeaders bool
}

func NewPrinter(out io.Writer, err io.Writer, opts PrinterOptions) *Printer {
	pretty := false
	if opts.ForcePretty {
		pretty = true
	} else if opts.ForceCompact {
		pretty = false
	} else {
		// auto
		if f, ok := out.(*os.File); ok {
			pretty = term.IsTerminal(int(f.Fd()))
		}
	}

	return &Printer{
		out: out,
		err: err,

		pretty: pretty,
		ndjson: opts.Ndjson,

		printStatus:  opts.PrintStatus,
		printHeaders: opts.PrintHeaders,
	}
}

func (p *Printer) Out() io.Writer      { return p.out }
func (p *Printer) Err() io.Writer      { return p.err }
func (p *Printer) NDJSONEnabled() bool { return p.ndjson }

func (p *Printer) PrintHTTP(status int, headers http.Header, body []byte) error {
	if p.printStatus {
		if _, err := fmt.Fprintf(p.err, "%d\n", status); err != nil {
			return err
		}
	}
	if p.printHeaders {
		for k, vv := range headers {
			printVal := strings.Join(vv, ", ")
			switch strings.ToLower(k) {
			case "authorization", "proxy-authorization", "set-cookie":
				printVal = "<redacted>"
			}
			if _, err := fmt.Fprintf(p.err, "%s: %s\n", k, printVal); err != nil {
				return err
			}
		}
	}

	return p.printBodyTo(p.out, body)
}

func (p *Printer) PrintBody(body []byte) error {
	return p.printBodyTo(p.out, body)
}

func (p *Printer) PrintHTTPError(status int, headers http.Header, body []byte) error {
	// Always print a status line for non-2xx responses.
	statusText := http.StatusText(status)
	if statusText != "" {
		if _, err := fmt.Fprintf(p.err, "HTTP %d %s\n", status, statusText); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintf(p.err, "HTTP %d\n", status); err != nil {
			return err
		}
	}

	if p.printHeaders {
		for k, vv := range headers {
			printVal := strings.Join(vv, ", ")
			switch strings.ToLower(k) {
			case "authorization", "proxy-authorization", "set-cookie":
				printVal = "<redacted>"
			}
			if _, err := fmt.Fprintf(p.err, "%s: %s\n", k, printVal); err != nil {
				return err
			}
		}
	}
	return p.printBodyTo(p.err, body)
}

func (p *Printer) printBodyTo(w io.Writer, body []byte) error {
	if len(body) == 0 {
		return nil
	}

	out := body
	if p.pretty && json.Valid(body) {
		var buf bytes.Buffer
		if err := json.Indent(&buf, body, "", "  "); err == nil {
			out = buf.Bytes()
		}
	}

	if _, err := w.Write(out); err != nil {
		return err
	}
	if len(out) == 0 || out[len(out)-1] != '\n' {
		_, _ = w.Write([]byte("\n"))
	}
	return nil
}
