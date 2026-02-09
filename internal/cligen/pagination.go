package cligen

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tarrence/mercury-cli/internal/mercuryhttp"
	"github.com/tarrence/mercury-cli/internal/openapi"
)

type paginationMode int

const (
	paginateNone paginationMode = iota
	paginateCursor
	paginatePageToken
	paginateOffset
)

type paginationPlan struct {
	mode paginationMode

	// queryParam is the request query parameter the pager mutates.
	queryParam string

	// itemField is the array field in the JSON response to accumulate.
	itemField string

	// nextTokenField is the response field that contains the token for the next page.
	// For cursor paging it is "page.nextPage".
	nextTokenField string

	// totalField is used for offset paging (e.g. "total").
	totalField string
}

type paginationResult struct {
	Items      []any
	LastObject map[string]any
	FirstTotal any

	LastStatus  int
	LastHeaders http.Header
}

func detectPaginationPlan(spec *openapi.Spec, op *openapi.Operation) *paginationPlan {
	if spec == nil || op == nil {
		return nil
	}

	hasQuery := func(name string) bool {
		for _, p := range op.Parameters {
			if strings.EqualFold(p.In, "query") && p.Name == name {
				return true
			}
		}
		return false
	}

	schema := jsonResponseSchema(spec, op, "200")
	if schema == nil {
		return nil
	}
	schema = spec.FlattenSchema(schema)
	if schema == nil || strings.EqualFold(schema.Type, "") {
		return nil
	}
	if !strings.EqualFold(schema.Type, "object") {
		return nil
	}

	// page_token style.
	if hasQuery("page_token") {
		if hasArrayProp(spec, schema, "records") && hasProp(schema, "next_page_token") {
			return &paginationPlan{
				mode:           paginatePageToken,
				queryParam:     "page_token",
				itemField:      "records",
				nextTokenField: "next_page_token",
				totalField:     "",
			}
		}
	}

	// offset style.
	if hasQuery("offset") {
		if hasArrayProp(spec, schema, "transactions") && hasProp(schema, "total") {
			return &paginationPlan{
				mode:       paginateOffset,
				queryParam: "offset",
				itemField:  "transactions",
				totalField: "total",
			}
		}
	}

	// cursor style: request has start_after; response has page.nextPage; plus a primary array property.
	if hasQuery("start_after") {
		pageSchema, ok := schema.Properties["page"]
		if ok {
			ps := pageSchema
			psF := spec.FlattenSchema(&ps)
			if psF != nil && strings.EqualFold(psF.Type, "object") {
				if _, ok := psF.Properties["nextPage"]; ok {
					itemField := firstArrayPropertyName(spec, schema, "page")
					if itemField != "" {
						return &paginationPlan{
							mode:           paginateCursor,
							queryParam:     "start_after",
							itemField:      itemField,
							nextTokenField: "page.nextPage",
						}
					}
				}
			}
		}
	}

	return nil
}

func fetchAll(plan *paginationPlan, initialQuery url.Values, maxPages int, sleep time.Duration, do func(url.Values) (*mercuryhttp.Result, error)) (*paginationResult, error) {
	if plan == nil || plan.mode == paginateNone {
		return nil, fmt.Errorf("missing pagination plan")
	}
	if maxPages <= 0 {
		maxPages = 1000
	}
	if initialQuery == nil {
		initialQuery = url.Values{}
	}

	q := cloneValues(initialQuery)

	res := &paginationResult{}

	offset := 0
	if plan.mode == paginateOffset {
		if v := q.Get(plan.queryParam); v != "" {
			if i, err := strconv.Atoi(v); err == nil && i >= 0 {
				offset = i
			}
		}
	}

	for page := 1; page <= maxPages; page++ {
		if plan.mode == paginateOffset {
			q.Set(plan.queryParam, strconv.Itoa(offset))
		}

		r, err := do(q)
		if err != nil {
			return nil, err
		}
		res.LastStatus = r.Status
		res.LastHeaders = r.Headers

		var v any
		if err := json.Unmarshal(r.Body, &v); err != nil {
			return nil, fmt.Errorf("parse JSON response: %w", err)
		}
		obj, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("unexpected JSON response type %T", v)
		}

		itemsVal, ok := obj[plan.itemField]
		if !ok {
			return nil, fmt.Errorf("response missing %q field", plan.itemField)
		}
		items, ok := itemsVal.([]any)
		if !ok {
			return nil, fmt.Errorf("response field %q is %T, expected array", plan.itemField, itemsVal)
		}

		res.Items = append(res.Items, items...)
		res.LastObject = obj

		switch plan.mode {
		case paginateCursor:
			next := cursorNextToken(obj)
			if next == "" {
				return res, nil
			}
			q.Set(plan.queryParam, next)
		case paginatePageToken:
			next := stringField(obj, plan.nextTokenField)
			if next == "" {
				return res, nil
			}
			q.Set(plan.queryParam, next)
		case paginateOffset:
			if res.FirstTotal == nil {
				res.FirstTotal = obj[plan.totalField]
			}
			offset += len(items)
			total := intFromAny(obj[plan.totalField])
			if total > 0 && offset >= total {
				return res, nil
			}
			if len(items) == 0 {
				return res, nil
			}
		default:
			return res, nil
		}

		if sleep > 0 {
			time.Sleep(sleep)
		}
	}

	return res, fmt.Errorf("pagination exceeded --max-pages=%d", maxPages)
}

func cloneValues(v url.Values) url.Values {
	out := url.Values{}
	for k, vv := range v {
		cp := append([]string(nil), vv...)
		out[k] = cp
	}
	return out
}

func hasProp(schema *openapi.Schema, name string) bool {
	if schema == nil {
		return false
	}
	_, ok := schema.Properties[name]
	return ok
}

func hasArrayProp(spec *openapi.Spec, schema *openapi.Schema, name string) bool {
	if schema == nil {
		return false
	}
	prop, ok := schema.Properties[name]
	if !ok {
		return false
	}
	p := prop
	pF := spec.FlattenSchema(&p)
	return pF != nil && strings.EqualFold(pF.Type, "array")
}

func firstArrayPropertyName(spec *openapi.Spec, schema *openapi.Schema, skip string) string {
	if schema == nil || schema.Properties == nil {
		return ""
	}
	keys := make([]string, 0, len(schema.Properties))
	for k := range schema.Properties {
		if k == skip {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		prop := schema.Properties[k]
		p := prop
		pF := spec.FlattenSchema(&p)
		if pF != nil && strings.EqualFold(pF.Type, "array") {
			return k
		}
	}
	return ""
}

func cursorNextToken(obj map[string]any) string {
	pageVal, ok := obj["page"]
	if !ok {
		return ""
	}
	pageObj, ok := pageVal.(map[string]any)
	if !ok {
		return ""
	}
	return stringField(pageObj, "nextPage")
}

func stringField(obj map[string]any, field string) string {
	v, ok := obj[field]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		return fmt.Sprint(v)
	}
}

func intFromAny(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case int64:
		return int(t)
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return int(i)
		}
		if f, err := t.Float64(); err == nil {
			return int(f)
		}
	case string:
		if i, err := strconv.Atoi(t); err == nil {
			return i
		}
	}
	return 0
}
