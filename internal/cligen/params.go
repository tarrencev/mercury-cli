package cligen

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tarrence/mercury-cli/internal/openapi"
)

type paramKind int

const (
	kindString paramKind = iota
	kindInt
	kindBool
	kindFloat
	kindStringArray
)

type paramBinding struct {
	param openapi.Parameter

	flagNames []string
	kind      paramKind

	s  *string
	i  *int
	b  *bool
	f  *float64
	sa *[]string
}

func bindParams(cmd *cobra.Command, spec *openapi.Spec, params []openapi.Parameter, where string) ([]*paramBinding, error) {
	var out []*paramBinding
	for _, p := range params {
		if p.Ref != "" {
			// None of the vendored specs currently use parameter $ref.
			return nil, fmt.Errorf("unsupported parameter $ref %q", p.Ref)
		}
		if strings.ToLower(p.In) != strings.ToLower(where) {
			continue
		}
		if p.Name == "" {
			continue
		}

		primary := kebabCase(p.Name)
		aliases := []string{}
		if primary != p.Name {
			aliases = append(aliases, p.Name)
		}

		kind := detectParamKind(spec, &p)
		desc := buildParamHelp(spec, &p, where, kind)

		binding := &paramBinding{
			param:     p,
			flagNames: append([]string{primary}, aliases...),
			kind:      kind,
			s:         new(string),
			i:         new(int),
			b:         new(bool),
			f:         new(float64),
			sa:        new([]string),
		}

		// Bind flags. Aliases share the same underlying variable; alias flags are hidden.
		switch kind {
		case kindString:
			cmd.Flags().StringVar(binding.s, primary, "", desc)
			for _, a := range aliases {
				cmd.Flags().StringVar(binding.s, a, "", "alias for --"+primary)
				_ = cmd.Flags().MarkHidden(a)
			}
		case kindInt:
			cmd.Flags().IntVar(binding.i, primary, 0, desc)
			for _, a := range aliases {
				cmd.Flags().IntVar(binding.i, a, 0, "alias for --"+primary)
				_ = cmd.Flags().MarkHidden(a)
			}
		case kindBool:
			cmd.Flags().BoolVar(binding.b, primary, false, desc)
			for _, a := range aliases {
				cmd.Flags().BoolVar(binding.b, a, false, "alias for --"+primary)
				_ = cmd.Flags().MarkHidden(a)
			}
		case kindFloat:
			cmd.Flags().Float64Var(binding.f, primary, 0, desc)
			for _, a := range aliases {
				cmd.Flags().Float64Var(binding.f, a, 0, "alias for --"+primary)
				_ = cmd.Flags().MarkHidden(a)
			}
		case kindStringArray:
			cmd.Flags().StringArrayVar(binding.sa, primary, nil, desc)
			for _, a := range aliases {
				cmd.Flags().StringArrayVar(binding.sa, a, nil, "alias for --"+primary)
				_ = cmd.Flags().MarkHidden(a)
			}
		default:
			cmd.Flags().StringVar(binding.s, primary, "", desc)
			for _, a := range aliases {
				cmd.Flags().StringVar(binding.s, a, "", "alias for --"+primary)
				_ = cmd.Flags().MarkHidden(a)
			}
		}

		if p.Required {
			_ = cmd.MarkFlagRequired(primary)
		}

		out = append(out, binding)
	}
	return out, nil
}

func buildParamHelp(spec *openapi.Spec, p *openapi.Parameter, where string, kind paramKind) string {
	if p == nil {
		return ""
	}

	desc := strings.TrimSpace(p.Description)
	if desc == "" && spec != nil && p.Schema != nil {
		s := spec.FlattenSchema(p.Schema)
		if s != nil {
			desc = strings.TrimSpace(s.Description)
		}
	}

	typeHint := paramTypeHint(spec, p)
	loc := strings.ToLower(where)

	if desc == "" {
		desc = fmt.Sprintf("%s parameter", titleFirst(loc))
		if typeHint != "" {
			desc += fmt.Sprintf(" (%s)", typeHint)
		}
		return desc
	}

	// Add a little extra context for repeatable flags.
	if kind == kindStringArray {
		desc += " (repeatable)"
	}
	return desc
}

func paramTypeHint(spec *openapi.Spec, p *openapi.Parameter) string {
	if spec == nil || p == nil || p.Schema == nil {
		return ""
	}
	s := spec.FlattenSchema(p.Schema)
	if s == nil {
		return ""
	}

	typ := strings.TrimSpace(s.Type)
	if strings.EqualFold(typ, "array") && s.Items != nil {
		item := spec.FlattenSchema(s.Items)
		if item != nil && item.Type != "" {
			typ = strings.TrimSpace(item.Type) + "[]"
			if item.Format != "" {
				return typ + " (" + strings.TrimSpace(item.Format) + ")"
			}
			return typ
		}
	}

	if typ == "" {
		return ""
	}
	if s.Format != "" {
		return typ + " (" + strings.TrimSpace(s.Format) + ")"
	}
	return typ
}

func titleFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func detectParamKind(spec *openapi.Spec, p *openapi.Parameter) paramKind {
	if spec == nil || p == nil || p.Schema == nil {
		return kindString
	}
	s := spec.FlattenSchema(p.Schema)
	if s == nil {
		return kindString
	}
	switch strings.ToLower(s.Type) {
	case "boolean":
		return kindBool
	case "integer":
		return kindInt
	case "number":
		return kindFloat
	case "array":
		return kindStringArray
	default:
		return kindString
	}
}

func (b *paramBinding) changed(cmd *cobra.Command) bool {
	for _, n := range b.flagNames {
		if cmd.Flags().Changed(n) {
			return true
		}
	}
	return false
}

func (b *paramBinding) valuesAsStrings() []string {
	switch b.kind {
	case kindString:
		return []string{*b.s}
	case kindInt:
		return []string{strconv.Itoa(*b.i)}
	case kindBool:
		return []string{strconv.FormatBool(*b.b)}
	case kindFloat:
		return []string{strconv.FormatFloat(*b.f, 'f', -1, 64)}
	case kindStringArray:
		if b.sa == nil {
			return nil
		}
		return append([]string(nil), (*b.sa)...)
	default:
		return []string{*b.s}
	}
}

func (b *paramBinding) addToQuery(values url.Values, cmd *cobra.Command) {
	if !b.changed(cmd) {
		return
	}
	for _, v := range b.valuesAsStrings() {
		values.Add(b.param.Name, v)
	}
}

func (b *paramBinding) addToHeaders(h map[string][]string, cmd *cobra.Command) {
	if !b.changed(cmd) {
		return
	}
	for _, v := range b.valuesAsStrings() {
		h[b.param.Name] = append(h[b.param.Name], v)
	}
}
