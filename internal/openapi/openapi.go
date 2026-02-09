package openapi

type SpecDoc struct {
	// Name is a stable identifier derived from the embedded filename (no extension).
	Name string
	// Filename is the embedded filename (basename).
	Filename string
	Spec     *Spec
}

// Spec is a minimal OpenAPI 3-ish model sufficient for generating CLI commands.
// Unknown JSON fields are ignored.
type Spec struct {
	OpenAPI string   `json:"openapi"`
	Info    Info     `json:"info"`
	Servers []Server `json:"servers"`

	Tags     []Tag                 `json:"tags,omitempty"`
	Security []map[string][]string `json:"security,omitempty"`

	Paths      map[string]PathItem `json:"paths"`
	Components Components          `json:"components,omitempty"`
}

type Info struct {
	Title   string `json:"title,omitempty"`
	Version string `json:"version,omitempty"`
}

type Tag struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

type Server struct {
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
}

type Components struct {
	Schemas         map[string]Schema `json:"schemas,omitempty"`
	SecuritySchemes map[string]any    `json:"securitySchemes,omitempty"`
}

type PathItem struct {
	Get     *Operation `json:"get,omitempty"`
	Post    *Operation `json:"post,omitempty"`
	Put     *Operation `json:"put,omitempty"`
	Delete  *Operation `json:"delete,omitempty"`
	Patch   *Operation `json:"patch,omitempty"`
	Head    *Operation `json:"head,omitempty"`
	Options *Operation `json:"options,omitempty"`
}

type Operation struct {
	OperationID string   `json:"operationId,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Summary     string   `json:"summary,omitempty"`
	Description string   `json:"description,omitempty"`

	Parameters  []Parameter           `json:"parameters,omitempty"`
	RequestBody *RequestBody          `json:"requestBody,omitempty"`
	Responses   map[string]Response   `json:"responses,omitempty"`
	Security    []map[string][]string `json:"security,omitempty"`
	Servers     []Server              `json:"servers,omitempty"`
}

type Parameter struct {
	Ref         string `json:"$ref,omitempty"`
	Name        string `json:"name,omitempty"`
	In          string `json:"in,omitempty"` // path, query, header, cookie
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`

	Schema  *Schema `json:"schema,omitempty"`
	Style   string  `json:"style,omitempty"`
	Explode *bool   `json:"explode,omitempty"`
}

type RequestBody struct {
	Ref      string               `json:"$ref,omitempty"`
	Required bool                 `json:"required,omitempty"`
	Content  map[string]MediaType `json:"content,omitempty"`
}

type Response struct {
	Ref         string               `json:"$ref,omitempty"`
	Description string               `json:"description,omitempty"`
	Content     map[string]MediaType `json:"content,omitempty"`
}

type MediaType struct {
	Schema *Schema `json:"schema,omitempty"`
}

type Schema struct {
	Ref        string            `json:"$ref,omitempty"`
	Type       string            `json:"type,omitempty"`
	Format     string            `json:"format,omitempty"`
	Nullable   bool              `json:"nullable,omitempty"`
	Enum       []any             `json:"enum,omitempty"`
	Items      *Schema           `json:"items,omitempty"`
	Properties map[string]Schema `json:"properties,omitempty"`
	Required   []string          `json:"required,omitempty"`
	AllOf      []*Schema         `json:"allOf,omitempty"`
	AnyOf      []*Schema         `json:"anyOf,omitempty"`
	OneOf      []*Schema         `json:"oneOf,omitempty"`
}
