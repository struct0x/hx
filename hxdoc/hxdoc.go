// Package hxdoc generates an OpenAPI 3.1.0 specification from routes registered with hx.HX
// and exposes it as a hx.HandlerFunc.
//
// Usage:
//
//	hx.HandleFunc("/openapi.json", hxdoc.Handler(server,
//	    hxdoc.WithTitle("Orders API"),
//	    hxdoc.WithVersion("1.0.0"),
//	    hxdoc.WithSecurityScheme("BearerAuth", hxdoc.BearerAuth()),
//	))
package hxdoc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/struct0x/hx"
)

// SecurityScheme defines an OpenAPI security scheme.
type SecurityScheme struct {
	Type         string `json:"type"`
	Scheme       string `json:"scheme,omitempty"`
	BearerFormat string `json:"bearerFormat,omitempty"`
	In           string `json:"in,omitempty"`
	Name         string `json:"name,omitempty"`
	Description  string `json:"description,omitempty"`
}

// BearerAuth returns a Bearer token security scheme.
func BearerAuth() SecurityScheme {
	return SecurityScheme{Type: "http", Scheme: "bearer"}
}

// APIKeyAuth returns an API key security scheme.
func APIKeyAuth(in, name string) SecurityScheme {
	return SecurityScheme{Type: "apiKey", In: in, Name: name}
}

type config struct {
	title           string
	version         string
	description     string
	servers         []apiServer
	securitySchemes map[string]SecurityScheme
}

// Opt configures the generated OpenAPI document.
type Opt func(*config)

// WithTitle sets the API title in the info object.
func WithTitle(title string) Opt {
	return func(c *config) { c.title = title }
}

// WithVersion sets the API version in the info object.
func WithVersion(version string) Opt {
	return func(c *config) { c.version = version }
}

// WithDescription sets the API description in the info object.
func WithDescription(desc string) Opt {
	return func(c *config) { c.description = desc }
}

// WithServer adds a server entry to the OpenAPI document.
func WithServer(url, description string) Opt {
	return func(c *config) {
		c.servers = append(c.servers, apiServer{URL: url, Description: description})
	}
}

// WithSecurityScheme adds a named security scheme to the components section.
func WithSecurityScheme(name string, scheme SecurityScheme) Opt {
	return func(c *config) {
		if c.securitySchemes == nil {
			c.securitySchemes = make(map[string]SecurityScheme)
		}
		c.securitySchemes[name] = scheme
	}
}

// UIHandler returns a hx.HandlerFunc that serves a Swagger UI HTML page
// loading the spec from specURL.
//
// Register it on a hx server:
//
//	server.HandleFunc("GET /docs", hxdoc.UIHandler("/openapi.json"))
func UIHandler(specURL string) hx.HandlerFunc {
	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
  <head>
    <title>API Docs</title>
    <meta charset="utf-8"/>
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
  </head>
  <body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
    <script>
      SwaggerUIBundle({
        url: %q,
        dom_id: '#swagger-ui',
        presets: [SwaggerUIBundle.presets.apis, SwaggerUIBundle.SwaggerUIStandalonePreset],
        layout: "BaseLayout",
        deepLinking: true,
      })
    </script>
  </body>
</html>`, specURL)

	return func(ctx context.Context, r *http.Request) error {
		w := hx.HijackResponseWriter(ctx)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, html)

		return nil
	}
}

// Handler returns an hx.HandlerFunc that serves the OpenAPI 3.1.0 spec as JSON.
// The spec is generated once at construction time from the routes registered on server.
// Only routes with an hx.Doc(...) option are included in the spec.
func Handler(server *hx.HX, opts ...Opt) hx.HandlerFunc {
	cfg := &config{
		title:   "API",
		version: "0.0.0",
	}
	for _, o := range opts {
		o(cfg)
	}

	spec := buildSpec(cfg, server.Routes())

	buf, buildErr := json.Marshal(spec)

	return func(ctx context.Context, r *http.Request) error {
		if buildErr != nil {
			return hx.Problem(
				http.StatusInternalServerError,
				"failed to build OpenAPI spec",
				hx.WithDetail(buildErr.Error()),
				hx.WithCause(buildErr),
			)
		}

		return hx.OK(bytes.NewReader(buf))
	}
}

type openAPIDoc struct {
	OpenAPI    string              `json:"openapi"`
	Info       apiInfo             `json:"info"`
	Servers    []apiServer         `json:"servers,omitempty"`
	Paths      map[string]pathItem `json:"paths,omitempty"`
	Components apiComponents       `json:"components,omitempty"`
}

type apiInfo struct {
	Title       string `json:"title"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
}

type apiServer struct {
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
}

type pathItem map[string]*operation

type operation struct {
	OperationID string                  `json:"operationId,omitempty"`
	Summary     string                  `json:"summary,omitempty"`
	Description string                  `json:"description,omitempty"`
	Tags        []string                `json:"tags,omitempty"`
	Deprecated  bool                    `json:"deprecated,omitempty"`
	Parameters  []parameter             `json:"parameters,omitempty"`
	RequestBody *requestBody            `json:"requestBody,omitempty"`
	Responses   map[string]*apiResponse `json:"responses"`
	Security    []map[string][]string   `json:"security,omitempty"`
}

type parameter struct {
	Name     string  `json:"name"`
	In       string  `json:"in"`
	Required bool    `json:"required,omitempty"`
	Schema   *Schema `json:"schema,omitempty"`
}

type requestBody struct {
	Required bool                  `json:"required,omitempty"`
	Content  map[string]*mediaType `json:"content"`
}

type mediaType struct {
	Schema *Schema `json:"schema,omitempty"`
}

type apiResponse struct {
	Description string                `json:"description"`
	Content     map[string]*mediaType `json:"content,omitempty"`
}

type apiComponents struct {
	Schemas         map[string]*Schema        `json:"schemas,omitempty"`
	SecuritySchemes map[string]SecurityScheme `json:"securitySchemes,omitempty"`
}

func buildSpec(cfg *config, routes []hx.RouteInfo) openAPIDoc {
	c := newCollector()
	// Seed with ProblemDetails so it always appears in the schemas panel.
	c.schemas["ProblemDetails"] = problemDetailsSchema()

	doc := openAPIDoc{
		OpenAPI: "3.1.0",
		Info: apiInfo{
			Title:       cfg.title,
			Version:     cfg.version,
			Description: cfg.description,
		},
		Servers: cfg.servers,
		Paths:   make(map[string]pathItem),
	}

	for _, route := range routes {
		if route.Doc == nil {
			continue
		}

		method := strings.ToLower(route.Method)
		if method == "" {
			method = "get"
		}

		item, ok := doc.Paths[route.Path]
		if !ok {
			item = pathItem{}
			doc.Paths[route.Path] = item
		}

		item[method] = buildOperation(c, route)
	}

	if len(doc.Paths) == 0 {
		doc.Paths = nil
	}

	doc.Components = apiComponents{
		Schemas:         c.schemas,
		SecuritySchemes: cfg.securitySchemes,
	}

	return doc
}

func buildOperation(c *collector, route hx.RouteInfo) *operation {
	d := route.Doc

	opID := d.OperationID
	if opID == "" {
		opID = deriveOperationID(route.Method, route.Path)
	}

	op := &operation{
		OperationID: opID,
		Summary:     d.Summary,
		Description: d.Description,
		Tags:        d.Tags,
		Deprecated:  d.Deprecated,
		Responses:   make(map[string]*apiResponse),
	}

	if len(d.Security) > 0 {
		req := make(map[string][]string, len(d.Security))
		for _, name := range d.Security {
			req[name] = []string{}
		}
		op.Security = []map[string][]string{req}
	}

	if d.Request != nil {
		t := reflect.TypeOf(d.Request)
		for t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		if t.Kind() == reflect.Struct {
			op.Parameters = c.extractParams(t)
			if bodySchema := c.extractBodySchema(t); bodySchema != nil {
				op.RequestBody = &requestBody{
					Required: true,
					Content:  map[string]*mediaType{"application/json": {Schema: bodySchema}},
				}
			}
		}
	}

	if d.Response != nil {
		s := c.schemaOf(reflect.TypeOf(d.Response))
		op.Responses["200"] = &apiResponse{
			Description: "OK",
			Content:     map[string]*mediaType{"application/json": {Schema: s}},
		}
	}

	for code, body := range d.Responses {
		key := strconv.Itoa(code)
		if body == nil {
			op.Responses[key] = &apiResponse{Description: http.StatusText(code)}
		} else {
			s := c.schemaOf(reflect.TypeOf(body))
			op.Responses[key] = &apiResponse{
				Description: http.StatusText(code),
				Content:     map[string]*mediaType{"application/json": {Schema: s}},
			}
		}
	}

	errorCodes := append([]int{http.StatusBadRequest, http.StatusInternalServerError}, d.Errors...)
	for _, code := range errorCodes {
		key := strconv.Itoa(code)
		if _, exists := op.Responses[key]; !exists {
			op.Responses[key] = &apiResponse{
				Description: http.StatusText(code),
				Content: map[string]*mediaType{
					"application/problem+json": {
						Schema: &Schema{Ref: "#/components/schemas/ProblemDetails"},
					},
				},
			}
		}
	}

	return op
}

func problemDetailsSchema() *Schema {
	return &Schema{
		Type: "object",
		Properties: map[string]*Schema{
			"type":     {Type: "string", Format: "uri"},
			"title":    {Type: "string"},
			"status":   {Type: "integer"},
			"detail":   {Type: "string"},
			"instance": {Type: "string", Format: "uri"},
		},
	}
}

func deriveOperationID(method, path string) string {
	method = strings.ToLower(method)
	path = strings.Trim(path, "/")
	path = strings.NewReplacer("{", "", "}", "", "/", "_").Replace(path)
	if method == "" {
		return path
	}
	if path == "" {
		return method
	}
	return method + "_" + path
}
