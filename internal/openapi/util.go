package openapi

import (
	"strings"
)

func (pi *PathItem) Operations() map[string]*Operation {
	out := map[string]*Operation{}
	if pi.Get != nil {
		out["GET"] = pi.Get
	}
	if pi.Post != nil {
		out["POST"] = pi.Post
	}
	if pi.Put != nil {
		out["PUT"] = pi.Put
	}
	if pi.Delete != nil {
		out["DELETE"] = pi.Delete
	}
	if pi.Patch != nil {
		out["PATCH"] = pi.Patch
	}
	if pi.Head != nil {
		out["HEAD"] = pi.Head
	}
	if pi.Options != nil {
		out["OPTIONS"] = pi.Options
	}
	return out
}

func (s *Spec) ServerURLForOperation(op *Operation) string {
	if op != nil && len(op.Servers) > 0 && op.Servers[0].URL != "" {
		return op.Servers[0].URL
	}
	if len(s.Servers) > 0 {
		return s.Servers[0].URL
	}
	return ""
}

func (s *Spec) OperationRequiresAuth(op *Operation) bool {
	// Operation-level security overrides global.
	if op != nil && op.Security != nil {
		return len(op.Security) > 0
	}
	return len(s.Security) > 0
}

func refSchemaName(ref string) (string, bool) {
	const prefix = "#/components/schemas/"
	if !strings.HasPrefix(ref, prefix) {
		return "", false
	}
	return strings.TrimPrefix(ref, prefix), true
}

func (s *Spec) ResolveSchemaRef(ref string) (*Schema, bool) {
	name, ok := refSchemaName(ref)
	if !ok {
		return nil, false
	}
	if s.Components.Schemas == nil {
		return nil, false
	}
	schema, ok := s.Components.Schemas[name]
	if !ok {
		return nil, false
	}
	cp := schema // copy so callers can mutate safely
	return &cp, true
}

func (s *Spec) DerefSchema(schema *Schema) *Schema {
	return s.derefSchema(schema, map[string]bool{})
}

func (s *Spec) derefSchema(schema *Schema, seen map[string]bool) *Schema {
	if schema == nil {
		return nil
	}
	if schema.Ref == "" {
		return schema
	}
	if seen[schema.Ref] {
		return schema
	}
	seen[schema.Ref] = true
	target, ok := s.ResolveSchemaRef(schema.Ref)
	if !ok {
		return schema
	}
	return s.derefSchema(target, seen)
}

// FlattenSchema tries to produce a schema with merged object properties by expanding
// $ref and allOf. This is intentionally conservative and only supports what the CLI needs.
func (s *Spec) FlattenSchema(schema *Schema) *Schema {
	return s.flattenSchema(schema, map[string]bool{})
}

func (s *Spec) flattenSchema(schema *Schema, seen map[string]bool) *Schema {
	if schema == nil {
		return nil
	}

	schema = s.derefSchema(schema, seen)
	if schema == nil {
		return nil
	}

	// If this is just an allOf wrapper, flatten the children.
	if len(schema.AllOf) > 0 && schema.Properties == nil && schema.Type == "" && schema.Items == nil {
		merged := &Schema{
			Type:       "object",
			Properties: map[string]Schema{},
		}
		for _, sub := range schema.AllOf {
			subF := s.flattenSchema(sub, seen)
			if subF == nil {
				continue
			}
			if subF.Type != "" && merged.Type == "" {
				merged.Type = subF.Type
			}
			for k, v := range subF.Properties {
				merged.Properties[k] = v
			}
		}
		if len(merged.Properties) == 0 {
			// Fall back to the original schema if we couldn't merge anything useful.
			return schema
		}
		return merged
	}

	// Preserve the schema, but deref immediate properties/items where possible.
	if schema.Type == "object" && len(schema.Properties) > 0 {
		cp := *schema
		cp.Properties = map[string]Schema{}
		for k, v := range schema.Properties {
			vv := v
			d := s.derefSchema(&vv, seen)
			if d != nil {
				cp.Properties[k] = *d
			} else {
				cp.Properties[k] = v
			}
		}
		return &cp
	}
	if schema.Type == "array" && schema.Items != nil {
		cp := *schema
		cp.Items = s.derefSchema(schema.Items, seen)
		return &cp
	}
	return schema
}
