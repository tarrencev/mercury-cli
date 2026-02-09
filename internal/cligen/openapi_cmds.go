package cligen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tarrence/mercury-cli/internal/mercuryhttp"
	"github.com/tarrence/mercury-cli/internal/openapi"
)

type genOp struct {
	specDocName string
	spec        *openapi.Spec

	method string
	path   string
	op     *openapi.Operation

	tag       string
	groupName string
	cmdName   string
}

func AddOpenAPICommands(root *cobra.Command, docs []*openapi.SpecDoc) error {
	var ops []genOp
	for _, doc := range docs {
		if doc == nil || doc.Spec == nil {
			continue
		}
		spec := doc.Spec

		paths := make([]string, 0, len(spec.Paths))
		for p := range spec.Paths {
			paths = append(paths, p)
		}
		sort.Strings(paths)

		for _, p := range paths {
			item := spec.Paths[p]
			opsByMethod := item.Operations()
			methods := make([]string, 0, len(opsByMethod))
			for m := range opsByMethod {
				methods = append(methods, m)
			}
			sort.Strings(methods)

			for _, method := range methods {
				op := opsByMethod[method]
				if op == nil {
					continue
				}
				if strings.TrimSpace(op.OperationID) == "" {
					return fmt.Errorf("%s %s %s missing operationId", doc.Filename, method, p)
				}

				tag := "misc"
				if len(op.Tags) > 0 && strings.TrimSpace(op.Tags[0]) != "" {
					tag = op.Tags[0]
				}
				groupName := kebabCase(tag)
				if groupName == "" {
					groupName = "misc"
				}

				ops = append(ops, genOp{
					specDocName: doc.Name,
					spec:        spec,
					method:      method,
					path:        p,
					op:          op,
					tag:         tag,
					groupName:   groupName,
					cmdName:     kebabCase(op.OperationID),
				})
			}
		}
	}

	sort.SliceStable(ops, func(i, j int) bool {
		if ops[i].groupName != ops[j].groupName {
			return ops[i].groupName < ops[j].groupName
		}
		if ops[i].cmdName != ops[j].cmdName {
			return ops[i].cmdName < ops[j].cmdName
		}
		if ops[i].method != ops[j].method {
			return ops[i].method < ops[j].method
		}
		return ops[i].path < ops[j].path
	})

	groupCmds := map[string]*cobra.Command{}
	seen := map[string]map[string]genOp{} // group -> cmdName -> op

	for _, g := range ops {
		group := groupCmds[g.groupName]
		if group == nil {
			group = &cobra.Command{
				Use:           g.groupName,
				Short:         g.tag,
				SilenceUsage:  true,
				SilenceErrors: true,
			}
			groupCmds[g.groupName] = group
			root.AddCommand(group)
			seen[g.groupName] = map[string]genOp{}
		}

		if _, ok := seen[g.groupName][g.cmdName]; ok {
			prev := seen[g.groupName][g.cmdName]
			return fmt.Errorf("duplicate command name %q in group %q (%s %s conflicts with %s %s)",
				g.cmdName, g.groupName, g.method, g.path, prev.method, prev.path)
		}
		seen[g.groupName][g.cmdName] = g

		opCmd, err := buildOperationCmd(g)
		if err != nil {
			return err
		}
		group.AddCommand(opCmd)
	}

	return nil
}

func buildOperationCmd(g genOp) (*cobra.Command, error) {
	spec := g.spec
	op := g.op

	pathParams := extractPathParams(g.path)
	use := g.cmdName
	for _, pp := range pathParams {
		use += " <" + pp + ">"
	}

	short := strings.TrimSpace(op.Summary)
	if short == "" {
		short = fmt.Sprintf("%s %s", g.method, g.path)
	}

	cmd := &cobra.Command{
		Use:           use,
		Short:         short,
		Long:          strings.TrimSpace(op.Description),
		Args:          cobra.ExactArgs(len(pathParams)),
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	queryBindings, err := bindParams(cmd, spec, op.Parameters, "query")
	if err != nil {
		return nil, err
	}
	headerBindings, err := bindParams(cmd, spec, op.Parameters, "header")
	if err != nil {
		return nil, err
	}

	var body *bodyFlags
	if op.RequestBody != nil {
		if op.RequestBody.Ref != "" {
			return nil, fmt.Errorf("unsupported requestBody $ref %q", op.RequestBody.Ref)
		}
		if len(op.RequestBody.Content) > 0 {
			cts := make([]string, 0, len(op.RequestBody.Content))
			for ct := range op.RequestBody.Content {
				cts = append(cts, ct)
			}
			sort.Strings(cts)
			body = bindBodyFlags(cmd, op.RequestBody.Required, cts)
		}
	}

	pagPlan := detectPaginationPlan(spec, op)
	allFlag := new(bool)
	maxPages := new(int)
	sleepMS := new(int)
	if pagPlan != nil {
		cmd.Flags().BoolVar(allFlag, "all", false, "Fetch all pages (for paginated list operations)")
		cmd.Flags().IntVar(maxPages, "max-pages", 1000, "Max pages to fetch with --all")
		cmd.Flags().IntVar(sleepMS, "sleep-ms", 0, "Sleep between pages when using --all")
	}

	requiresAuth := spec.OperationRequiresAuth(op)
	method := g.method
	pathTemplate := g.path

	// Copy captured values for closure safety.
	specDocName := g.specDocName
	tag := g.tag
	cmdName := g.cmdName

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		rt, err := RuntimeFrom(cmd)
		if err != nil {
			return err
		}
		if requiresAuth && strings.TrimSpace(rt.Token) == "" {
			// The error body is likely the most useful output; print a clear hint too.
			fmt.Fprintf(rt.Printer.Err(), "Missing token for %s/%s %s %s. Set MERCURY_TOKEN or pass --token.\n", tag, cmdName, method, pathTemplate)
			return fmt.Errorf("missing token")
		}

		baseURL := strings.TrimSpace(rt.BaseURL)
		if baseURL == "" {
			baseURL = strings.TrimSpace(spec.ServerURLForOperation(op))
			if baseURL == "" {
				return fmt.Errorf("no server URL found for %s %s (%s)", method, pathTemplate, specDocName)
			}
			baseURL, err = applyEnvToServerURL(baseURL, rt.Env)
			if err != nil {
				return err
			}
		}

		expandedPath := pathTemplate
		for i, name := range pathParams {
			expandedPath = strings.ReplaceAll(expandedPath, "{"+name+"}", url.PathEscape(args[i]))
		}

		baseEndpoint, err := joinBaseAndPath(baseURL, expandedPath)
		if err != nil {
			return err
		}

		q := url.Values{}
		for _, b := range queryBindings {
			b.addToQuery(q, cmd)
		}
		h := http.Header{}
		for _, b := range headerBindings {
			b.addToHeaders(h, cmd)
		}

		do := func(query url.Values) (*mercuryhttp.Result, error) {
			endpoint := baseEndpoint
			if len(query) > 0 {
				u, err := url.Parse(endpoint)
				if err != nil {
					return nil, err
				}
				u.RawQuery = query.Encode()
				endpoint = u.String()
			}

			var reqBody []byte
			ct := ""
			if body != nil {
				reqBody, ct, err = body.build(cmd)
				if err != nil {
					return nil, err
				}
			}

			var req *http.Request
			if len(reqBody) > 0 {
				req, err = http.NewRequestWithContext(cmd.Context(), method, endpoint, bytes.NewReader(reqBody))
				if err != nil {
					return nil, err
				}
				req.GetBody = func() (io.ReadCloser, error) {
					return io.NopCloser(bytes.NewReader(reqBody)), nil
				}
				if ct != "" {
					req.Header.Set("Content-Type", ct)
				}
			} else {
				req, err = http.NewRequestWithContext(cmd.Context(), method, endpoint, nil)
				if err != nil {
					return nil, err
				}
			}

			for k, vv := range h {
				for _, v := range vv {
					req.Header.Add(k, v)
				}
			}

			// Apply auth when a token is present, even if the spec does not mark the operation as secured.
			// The spec security metadata isn't always complete (e.g., some onboarding endpoints).
			if strings.TrimSpace(rt.Token) != "" {
				mercuryhttp.ApplyAuth(req, rt.Token, rt.Auth)
			}

			res, err := rt.Client.Do(req, reqBody)
			if err != nil {
				return nil, err
			}
			if res.Status >= 400 {
				_ = rt.Printer.PrintHTTPError(res.Status, res.Headers, res.Body)
				return nil, fmt.Errorf("HTTP %d", res.Status)
			}
			return res, nil
		}

		if pagPlan != nil && *allFlag {
			if method != http.MethodGet {
				return fmt.Errorf("--all is only supported for GET operations")
			}
			sleep := time.Duration(*sleepMS) * time.Millisecond
			pres, err := fetchAll(pagPlan, q, *maxPages, sleep, do)
			if err != nil {
				return err
			}

			// Print status/headers (if enabled) once for the final successful page.
			if err := rt.Printer.PrintHTTP(pres.LastStatus, pres.LastHeaders, nil); err != nil {
				return err
			}

			if rt.Printer.NDJSONEnabled() {
				for _, item := range pres.Items {
					line, err := json.Marshal(item)
					if err != nil {
						return err
					}
					if _, err := rt.Printer.Out().Write(append(line, '\n')); err != nil {
						return err
					}
				}
				return nil
			}

			outObj := pres.LastObject
			if outObj == nil {
				outObj = map[string]any{}
			}
			outObj[pagPlan.itemField] = pres.Items
			if pagPlan.mode == paginateOffset && pagPlan.totalField != "" && pres.FirstTotal != nil {
				outObj[pagPlan.totalField] = pres.FirstTotal
			}
			b, err := json.Marshal(outObj)
			if err != nil {
				return err
			}
			return rt.Printer.PrintBody(b)
		}

		res, err := do(q)
		if err != nil {
			return err
		}
		return rt.Printer.PrintHTTP(res.Status, res.Headers, res.Body)
	}

	return cmd, nil
}

func jsonResponseSchema(spec *openapi.Spec, op *openapi.Operation, statusCode string) *openapi.Schema {
	if spec == nil || op == nil {
		return nil
	}
	resp, ok := op.Responses[statusCode]
	if !ok {
		return nil
	}
	for ct, mt := range resp.Content {
		if strings.HasPrefix(ct, "application/json") {
			return mt.Schema
		}
	}
	for _, mt := range resp.Content {
		return mt.Schema
	}
	return nil
}
