package hxdoc

import (
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/struct0x/hx"
)

func ptr[T any](v T) *T { return &v }

func problemRef() *Schema {
	return &Schema{Ref: "#/components/schemas/ProblemDetails"}
}

func problemResp(code int) *apiResponse {
	return &apiResponse{
		Description: http.StatusText(code),
		Content:     map[string]*mediaType{"application/problem+json": {Schema: problemRef()}},
	}
}

func TestBuildSpec(t *testing.T) {
	type bodyReq struct {
		Name string `json:"name" validate:"required"`
		Age  int    `json:"age"  validate:"gte=0"`
	}
	type paramReq struct {
		ID      string `path:"id"      validate:"required"`
		Verbose bool   `query:"verbose"`
	}
	type orderResp struct {
		ID string `json:"id"`
	}
	type address struct {
		Street string `json:"street"`
		City   string `json:"city"`
	}
	type person struct {
		Name    string  `json:"name"`
		Address address `json:"address"`
	}
	type mixedReq struct {
		ID   string   `path:"id"   validate:"required"`
		Name string   `json:"name" validate:"required"`
		Tags []string `json:"tags"`
	}
	type customErr struct {
		Message string `json:"message"`
	}

	baseCfg := func() *config { return &config{title: "API", version: "1.0"} }
	baseCmps := func() apiComponents {
		return apiComponents{Schemas: map[string]*Schema{"ProblemDetails": problemDetailsSchema()}}
	}
	orderRespSchema := func() *Schema {
		return &Schema{Type: "object", Properties: map[string]*Schema{"id": {Type: "string"}}}
	}

	tests := []struct {
		name   string
		cfg    *config
		routes []hx.RouteInfo
		want   openAPIDoc
	}{
		{
			name:   "no_documented_routes",
			cfg:    baseCfg(),
			routes: nil,
			want: openAPIDoc{
				OpenAPI:    "3.1.0",
				Info:       apiInfo{Title: "API", Version: "1.0"},
				Components: baseCmps(),
			},
		},
		{
			name: "undocumented_route_omitted",
			cfg:  baseCfg(),
			routes: []hx.RouteInfo{
				{Method: "GET", Path: "/ping"},
			},
			want: openAPIDoc{
				OpenAPI:    "3.1.0",
				Info:       apiInfo{Title: "API", Version: "1.0"},
				Components: baseCmps(),
			},
		},
		{
			name: "derived_operation_id",
			cfg:  baseCfg(),
			routes: []hx.RouteInfo{
				{Method: "POST", Path: "/orders", Doc: &hx.RouteDoc{Summary: "Create order"}},
			},
			want: openAPIDoc{
				OpenAPI: "3.1.0",
				Info:    apiInfo{Title: "API", Version: "1.0"},
				Paths: map[string]pathItem{
					"/orders": {"post": {
						OperationID: "post_orders",
						Summary:     "Create order",
						Responses: map[string]*apiResponse{
							"400": problemResp(http.StatusBadRequest),
							"500": problemResp(http.StatusInternalServerError),
						},
					}},
				},
				Components: baseCmps(),
			},
		},
		{
			name: "explicit_operation_id",
			cfg:  baseCfg(),
			routes: []hx.RouteInfo{
				{Method: "GET", Path: "/orders/{id}", Doc: &hx.RouteDoc{OperationID: "fetchOrder"}},
			},
			want: openAPIDoc{
				OpenAPI: "3.1.0",
				Info:    apiInfo{Title: "API", Version: "1.0"},
				Paths: map[string]pathItem{
					"/orders/{id}": {"get": {
						OperationID: "fetchOrder",
						Responses: map[string]*apiResponse{
							"400": problemResp(http.StatusBadRequest),
							"500": problemResp(http.StatusInternalServerError),
						},
					}},
				},
				Components: baseCmps(),
			},
		},
		{
			name: "request_body",
			cfg:  baseCfg(),
			routes: []hx.RouteInfo{
				{Method: "POST", Path: "/items", Doc: &hx.RouteDoc{Request: bodyReq{}}},
			},
			want: openAPIDoc{
				OpenAPI: "3.1.0",
				Info:    apiInfo{Title: "API", Version: "1.0"},
				Paths: map[string]pathItem{
					"/items": {"post": {
						OperationID: "post_items",
						RequestBody: &requestBody{
							Required: true,
							Content: map[string]*mediaType{
								"application/json": {Schema: &Schema{
									Type: "object",
									Properties: map[string]*Schema{
										"name": {Type: "string"},
										"age":  {Type: "integer", Minimum: ptr(0.0)},
									},
									Required: []string{"name"},
								}},
							},
						},
						Responses: map[string]*apiResponse{
							"400": problemResp(http.StatusBadRequest),
							"500": problemResp(http.StatusInternalServerError),
						},
					}},
				},
				Components: baseCmps(),
			},
		},
		{
			name: "path_and_query_params",
			cfg:  baseCfg(),
			routes: []hx.RouteInfo{
				{Method: "GET", Path: "/orders/{id}", Doc: &hx.RouteDoc{Request: paramReq{}}},
			},
			want: openAPIDoc{
				OpenAPI: "3.1.0",
				Info:    apiInfo{Title: "API", Version: "1.0"},
				Paths: map[string]pathItem{
					"/orders/{id}": {"get": {
						OperationID: "get_orders_id",
						Parameters: []parameter{
							{Name: "id", In: "path", Required: true, Schema: &Schema{Type: "string"}},
							{Name: "verbose", In: "query", Schema: &Schema{Type: "boolean"}},
						},
						Responses: map[string]*apiResponse{
							"400": problemResp(http.StatusBadRequest),
							"500": problemResp(http.StatusInternalServerError),
						},
					}},
				},
				Components: baseCmps(),
			},
		},
		{
			name: "success_response_registers_schema",
			cfg:  baseCfg(),
			routes: []hx.RouteInfo{
				{Method: "GET", Path: "/orders/{id}", Doc: &hx.RouteDoc{
					OperationID: "getOrder",
					Response:    orderResp{},
				}},
			},
			want: openAPIDoc{
				OpenAPI: "3.1.0",
				Info:    apiInfo{Title: "API", Version: "1.0"},
				Paths: map[string]pathItem{
					"/orders/{id}": {"get": {
						OperationID: "getOrder",
						Responses: map[string]*apiResponse{
							"200": {
								Description: "OK",
								Content: map[string]*mediaType{
									"application/json": {Schema: &Schema{Ref: "#/components/schemas/orderResp"}},
								},
							},
							"400": problemResp(http.StatusBadRequest),
							"500": problemResp(http.StatusInternalServerError),
						},
					}},
				},
				Components: apiComponents{
					Schemas: map[string]*Schema{
						"ProblemDetails": problemDetailsSchema(),
						"orderResp":      orderRespSchema(),
					},
				},
			},
		},
		{
			name: "additional_responses",
			cfg:  baseCfg(),
			routes: []hx.RouteInfo{
				{Method: "POST", Path: "/orders", Doc: &hx.RouteDoc{
					OperationID: "createOrder",
					Responses: map[int]any{
						http.StatusCreated:   orderResp{},
						http.StatusNoContent: nil,
					},
				}},
			},
			want: openAPIDoc{
				OpenAPI: "3.1.0",
				Info:    apiInfo{Title: "API", Version: "1.0"},
				Paths: map[string]pathItem{
					"/orders": {"post": {
						OperationID: "createOrder",
						Responses: map[string]*apiResponse{
							"201": {
								Description: "Created",
								Content: map[string]*mediaType{
									"application/json": {Schema: &Schema{Ref: "#/components/schemas/orderResp"}},
								},
							},
							"204": {Description: "No Content"},
							"400": problemResp(http.StatusBadRequest),
							"500": problemResp(http.StatusInternalServerError),
						},
					}},
				},
				Components: apiComponents{
					Schemas: map[string]*Schema{
						"ProblemDetails": problemDetailsSchema(),
						"orderResp":      orderRespSchema(),
					},
				},
			},
		},
		{
			name: "extra_error_codes",
			cfg:  baseCfg(),
			routes: []hx.RouteInfo{
				{Method: "DELETE", Path: "/orders/{id}", Doc: &hx.RouteDoc{
					OperationID: "deleteOrder",
					Errors:      []int{http.StatusNotFound, http.StatusForbidden},
				}},
			},
			want: openAPIDoc{
				OpenAPI: "3.1.0",
				Info:    apiInfo{Title: "API", Version: "1.0"},
				Paths: map[string]pathItem{
					"/orders/{id}": {"delete": {
						OperationID: "deleteOrder",
						Responses: map[string]*apiResponse{
							"400": problemResp(http.StatusBadRequest),
							"500": problemResp(http.StatusInternalServerError),
							"404": problemResp(http.StatusNotFound),
							"403": problemResp(http.StatusForbidden),
						},
					}},
				},
				Components: baseCmps(),
			},
		},
		{
			name: "security_scheme_and_operation",
			cfg: &config{
				title:   "API",
				version: "1.0",
				securitySchemes: map[string]SecurityScheme{
					"BearerAuth": BearerAuth(),
				},
			},
			routes: []hx.RouteInfo{
				{Method: "POST", Path: "/orders", Doc: &hx.RouteDoc{
					OperationID: "createOrder",
					Security:    []string{"BearerAuth"},
				}},
			},
			want: openAPIDoc{
				OpenAPI: "3.1.0",
				Info:    apiInfo{Title: "API", Version: "1.0"},
				Paths: map[string]pathItem{
					"/orders": {"post": {
						OperationID: "createOrder",
						Security:    []map[string][]string{{"BearerAuth": {}}},
						Responses: map[string]*apiResponse{
							"400": problemResp(http.StatusBadRequest),
							"500": problemResp(http.StatusInternalServerError),
						},
					}},
				},
				Components: apiComponents{
					Schemas: map[string]*Schema{"ProblemDetails": problemDetailsSchema()},
					SecuritySchemes: map[string]SecurityScheme{
						"BearerAuth": BearerAuth(),
					},
				},
			},
		},
		{
			name: "tags_and_deprecated",
			cfg:  baseCfg(),
			routes: []hx.RouteInfo{
				{Method: "GET", Path: "/old", Doc: &hx.RouteDoc{
					OperationID: "oldEndpoint",
					Tags:        []string{"legacy"},
					Deprecated:  true,
				}},
			},
			want: openAPIDoc{
				OpenAPI: "3.1.0",
				Info:    apiInfo{Title: "API", Version: "1.0"},
				Paths: map[string]pathItem{
					"/old": {"get": {
						OperationID: "oldEndpoint",
						Tags:        []string{"legacy"},
						Deprecated:  true,
						Responses: map[string]*apiResponse{
							"400": problemResp(http.StatusBadRequest),
							"500": problemResp(http.StatusInternalServerError),
						},
					}},
				},
				Components: baseCmps(),
			},
		},
		{
			name: "config_info_and_servers",
			cfg: &config{
				title:       "Orders API",
				version:     "2.0.0",
				description: "Manages orders",
				servers:     []apiServer{{URL: "https://api.example.com", Description: "Production"}},
			},
			routes: nil,
			want: openAPIDoc{
				OpenAPI:    "3.1.0",
				Info:       apiInfo{Title: "Orders API", Version: "2.0.0", Description: "Manages orders"},
				Servers:    []apiServer{{URL: "https://api.example.com", Description: "Production"}},
				Components: baseCmps(),
			},
		},
		{
			// Struct with both path params and json body fields: params go to Parameters,
			// json-only fields go to RequestBody.
			name: "mixed_request_params_and_body",
			cfg:  baseCfg(),
			routes: []hx.RouteInfo{
				{Method: "PATCH", Path: "/items/{id}", Doc: &hx.RouteDoc{Request: mixedReq{}}},
			},
			want: openAPIDoc{
				OpenAPI: "3.1.0",
				Info:    apiInfo{Title: "API", Version: "1.0"},
				Paths: map[string]pathItem{
					"/items/{id}": {"patch": {
						OperationID: "patch_items_id",
						Parameters: []parameter{
							{Name: "id", In: "path", Required: true, Schema: &Schema{Type: "string"}},
						},
						RequestBody: &requestBody{
							Required: true,
							Content: map[string]*mediaType{
								"application/json": {Schema: &Schema{
									Type: "object",
									Properties: map[string]*Schema{
										"name": {Type: "string"},
										"tags": {Type: "array", Items: &Schema{Type: "string"}},
									},
									Required: []string{"name"},
								}},
							},
						},
						Responses: map[string]*apiResponse{
							"400": problemResp(http.StatusBadRequest),
							"500": problemResp(http.StatusInternalServerError),
						},
					}},
				},
				Components: baseCmps(),
			},
		},
		{
			// Empty method string falls back to "get" in the paths map key.
			// deriveOperationID("", "/health") → path-only id "health".
			name: "empty_method_defaults_to_get",
			cfg:  baseCfg(),
			routes: []hx.RouteInfo{
				{Method: "", Path: "/health", Doc: &hx.RouteDoc{}},
			},
			want: openAPIDoc{
				OpenAPI: "3.1.0",
				Info:    apiInfo{Title: "API", Version: "1.0"},
				Paths: map[string]pathItem{
					"/health": {"get": {
						OperationID: "health",
						Responses: map[string]*apiResponse{
							"400": problemResp(http.StatusBadRequest),
							"500": problemResp(http.StatusInternalServerError),
						},
					}},
				},
				Components: baseCmps(),
			},
		},
		{
			// Two routes on the same path are merged into a single pathItem entry.
			name: "two_routes_same_path",
			cfg:  baseCfg(),
			routes: []hx.RouteInfo{
				{Method: "GET", Path: "/orders", Doc: &hx.RouteDoc{OperationID: "listOrders"}},
				{Method: "POST", Path: "/orders", Doc: &hx.RouteDoc{OperationID: "createOrder"}},
			},
			want: openAPIDoc{
				OpenAPI: "3.1.0",
				Info:    apiInfo{Title: "API", Version: "1.0"},
				Paths: map[string]pathItem{
					"/orders": {
						"get": {
							OperationID: "listOrders",
							Responses: map[string]*apiResponse{
								"400": problemResp(http.StatusBadRequest),
								"500": problemResp(http.StatusInternalServerError),
							},
						},
						"post": {
							OperationID: "createOrder",
							Responses: map[string]*apiResponse{
								"400": problemResp(http.StatusBadRequest),
								"500": problemResp(http.StatusInternalServerError),
							},
						},
					},
				},
				Components: baseCmps(),
			},
		},
		{
			// An explicit response for 400 set via Responses map is not clobbered
			// by the implicit ProblemDetails 400 — the check is "if not exists".
			name: "explicit_response_not_overridden_by_implicit_error",
			cfg:  baseCfg(),
			routes: []hx.RouteInfo{
				{Method: "GET", Path: "/foo", Doc: &hx.RouteDoc{
					OperationID: "getFoo",
					Responses: map[int]any{
						http.StatusBadRequest: customErr{},
					},
				}},
			},
			want: openAPIDoc{
				OpenAPI: "3.1.0",
				Info:    apiInfo{Title: "API", Version: "1.0"},
				Paths: map[string]pathItem{
					"/foo": {"get": {
						OperationID: "getFoo",
						Responses: map[string]*apiResponse{
							"400": {
								Description: "Bad Request",
								Content: map[string]*mediaType{
									"application/json": {Schema: &Schema{Ref: "#/components/schemas/customErr"}},
								},
							},
							"500": problemResp(http.StatusInternalServerError),
						},
					}},
				},
				Components: apiComponents{
					Schemas: map[string]*Schema{
						"ProblemDetails": problemDetailsSchema(),
						"customErr": {
							Type:       "object",
							Properties: map[string]*Schema{"message": {Type: "string"}},
						},
					},
				},
			},
		},
		{
			// A response struct that contains another named struct: both types are
			// registered in components/schemas, and the outer references the inner via $ref.
			name: "nested_struct_registers_all_schemas",
			cfg:  baseCfg(),
			routes: []hx.RouteInfo{
				{Method: "GET", Path: "/person", Doc: &hx.RouteDoc{
					OperationID: "getPerson",
					Response:    person{},
				}},
			},
			want: openAPIDoc{
				OpenAPI: "3.1.0",
				Info:    apiInfo{Title: "API", Version: "1.0"},
				Paths: map[string]pathItem{
					"/person": {"get": {
						OperationID: "getPerson",
						Responses: map[string]*apiResponse{
							"200": {
								Description: "OK",
								Content: map[string]*mediaType{
									"application/json": {Schema: &Schema{Ref: "#/components/schemas/person"}},
								},
							},
							"400": problemResp(http.StatusBadRequest),
							"500": problemResp(http.StatusInternalServerError),
						},
					}},
				},
				Components: apiComponents{
					Schemas: map[string]*Schema{
						"ProblemDetails": problemDetailsSchema(),
						"person": {
							Type: "object",
							Properties: map[string]*Schema{
								"name":    {Type: "string"},
								"address": {Ref: "#/components/schemas/address"},
							},
						},
						"address": {
							Type: "object",
							Properties: map[string]*Schema{
								"street": {Type: "string"},
								"city":   {Type: "string"},
							},
						},
					},
				},
			},
		},
		{
			// Passing a pointer to a request struct is equivalent to passing the struct value:
			// buildOperation dereferences pointer types before inspection.
			name: "pointer_request_type",
			cfg:  baseCfg(),
			routes: []hx.RouteInfo{
				{Method: "POST", Path: "/items", Doc: &hx.RouteDoc{Request: (*bodyReq)(nil)}},
			},
			want: openAPIDoc{
				OpenAPI: "3.1.0",
				Info:    apiInfo{Title: "API", Version: "1.0"},
				Paths: map[string]pathItem{
					"/items": {"post": {
						OperationID: "post_items",
						RequestBody: &requestBody{
							Required: true,
							Content: map[string]*mediaType{
								"application/json": {Schema: &Schema{
									Type: "object",
									Properties: map[string]*Schema{
										"name": {Type: "string"},
										"age":  {Type: "integer", Minimum: ptr(0.0)},
									},
									Required: []string{"name"},
								}},
							},
						},
						Responses: map[string]*apiResponse{
							"400": problemResp(http.StatusBadRequest),
							"500": problemResp(http.StatusInternalServerError),
						},
					}},
				},
				Components: baseCmps(),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildSpec(tc.cfg, tc.routes)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestApplyValidate exercises every branch of applyValidate, covering all
// format, pattern, numeric, enum, and constraint mappings from go-playground/validator tags.
func TestApplyValidate(t *testing.T) {
	strType := reflect.TypeOf("")
	intType := reflect.TypeOf(0)
	floatType := reflect.TypeOf(0.0)
	sliceType := reflect.TypeOf([]string{})

	tests := []struct {
		name string
		tag  string
		typ  reflect.Type
		want Schema
	}{
		// ── format tags ──────────────────────────────────────────────────────
		{"email", "email", strType, Schema{Format: "email"}},
		{"url", "url", strType, Schema{Format: "uri"}},
		{"uri", "uri", strType, Schema{Format: "uri"}},
		{"uuid", "uuid", strType, Schema{Format: "uuid"}},
		{"uuid3", "uuid3", strType, Schema{Format: "uuid"}},
		{"uuid4", "uuid4", strType, Schema{Format: "uuid"}},
		{"uuid5", "uuid5", strType, Schema{Format: "uuid"}},
		{"hostname", "hostname", strType, Schema{Format: "hostname"}},
		{"hostname_rfc1123", "hostname_rfc1123", strType, Schema{Format: "hostname"}},
		{"fqdn", "fqdn", strType, Schema{Format: "hostname"}},
		{"ip", "ip", strType, Schema{Format: "ip"}},
		{"ip_addr", "ip_addr", strType, Schema{Format: "ip"}},
		{"ipv4", "ipv4", strType, Schema{Format: "ipv4"}},
		{"ip4_addr", "ip4_addr", strType, Schema{Format: "ipv4"}},
		{"ipv6", "ipv6", strType, Schema{Format: "ipv6"}},
		{"ip6_addr", "ip6_addr", strType, Schema{Format: "ipv6"}},
		{"cidr", "cidr", strType, Schema{Format: "cidr"}},
		{"cidrv4", "cidrv4", strType, Schema{Format: "cidr"}},
		{"cidrv6", "cidrv6", strType, Schema{Format: "cidr"}},
		{"mac", "mac", strType, Schema{Format: "mac-address"}},
		{"base64", "base64", strType, Schema{Format: "byte"}},
		{"base64url", "base64url", strType, Schema{Format: "byte"}},
		{"base64rawurl", "base64rawurl", strType, Schema{Format: "byte"}},
		{"base32", "base32", strType, Schema{Format: "base32"}},
		{"jwt", "jwt", strType, Schema{Format: "jwt"}},
		{"semver", "semver", strType, Schema{Format: "semver"}},
		{"credit_card", "credit_card", strType, Schema{Format: "credit-card"}},
		{"iso3166_1_alpha2", "iso3166_1_alpha2", strType, Schema{Format: "iso3166-1-alpha-2"}},
		{"iso3166_1_alpha3", "iso3166_1_alpha3", strType, Schema{Format: "iso3166-1-alpha-3"}},
		{"bcp47_language_tag", "bcp47_language_tag", strType, Schema{Format: "bcp47-language-tag"}},
		{"timezone", "timezone", strType, Schema{Format: "timezone"}},
		{"datetime_prefix", "datetime=2006-01-02", strType, Schema{Format: "date-time"}},

		// ── pattern tags ─────────────────────────────────────────────────────
		{"alpha", "alpha", strType, Schema{Pattern: "^[a-zA-Z]+$"}},
		{"alphaspace", "alphaspace", strType, Schema{Pattern: "^[a-zA-Z ]+$"}},
		{"alphanum", "alphanum", strType, Schema{Pattern: "^[a-zA-Z0-9]+$"}},
		{"numeric", "numeric", strType, Schema{Pattern: "^[0-9]+$"}},
		{"number", "number", strType, Schema{Pattern: "^[-+]?[0-9]*\\.?[0-9]+$"}},
		{"hexadecimal", "hexadecimal", strType, Schema{Pattern: "^(0[xX])?[0-9a-fA-F]+$"}},
		{"hexcolor", "hexcolor", strType, Schema{Pattern: "^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$"}},
		{"e164", "e164", strType, Schema{Pattern: `^\+[1-9]\d{1,14}$`}},

		// ── exclusive / inclusive numeric bounds ─────────────────────────────
		{"gte", "gte=5", floatType, Schema{Minimum: ptr(5.0)}},
		{"lte", "lte=10", floatType, Schema{Maximum: ptr(10.0)}},
		{"gt", "gt=0", floatType, Schema{ExclusiveMinimum: ptr(0.0)}},
		{"lt", "lt=100", floatType, Schema{ExclusiveMaximum: ptr(100.0)}},

		// ── min= / max= / len= dispatch by type ──────────────────────────────
		{"min_string", "min=3", strType, Schema{MinLength: ptr(3)}},
		{"max_string", "max=50", strType, Schema{MaxLength: ptr(50)}},
		{"len_string", "len=10", strType, Schema{MinLength: ptr(10), MaxLength: ptr(10)}},
		{"min_slice", "min=1", sliceType, Schema{MinItems: ptr(1)}},
		{"max_slice", "max=5", sliceType, Schema{MaxItems: ptr(5)}},
		{"min_int", "min=0", intType, Schema{Minimum: ptr(0.0)}},
		{"max_int", "max=100", intType, Schema{Maximum: ptr(100.0)}},

		// ── well-known numeric range shortcuts ───────────────────────────────
		{"port", "port", intType, Schema{Minimum: ptr(1.0), Maximum: ptr(65535.0)}},
		{"latitude", "latitude", floatType, Schema{Minimum: ptr(-90.0), Maximum: ptr(90.0)}},
		{"longitude", "longitude", floatType, Schema{Minimum: ptr(-180.0), Maximum: ptr(180.0)}},

		// ── enum: oneof= / oneofci= / eq= ────────────────────────────────────
		{
			"oneof_string", "oneof=active pending cancelled", strType,
			Schema{Enum: []any{"active", "pending", "cancelled"}},
		},
		{
			"oneof_int", "oneof=1 2 3", intType,
			Schema{Enum: []any{int64(1), int64(2), int64(3)}},
		},
		{
			"oneof_float", "oneof=1.5 2.5", floatType,
			Schema{Enum: []any{1.5, 2.5}},
		},
		{
			"oneofci", "oneofci=x y", strType,
			Schema{Enum: []any{"x", "y"}},
		},
		{
			"eq_string", "eq=hello", strType,
			Schema{Enum: []any{"hello"}},
		},
		{
			"eq_int", "eq=42", intType,
			Schema{Enum: []any{int64(42)}},
		},

		// ── uniqueItems ───────────────────────────────────────────────────────
		{"unique_slice", "unique", sliceType, Schema{UniqueItems: true}},
		{"unique_string_noop", "unique", strType, Schema{}}, // no effect on non-collection

		// ── multi-part tags are all applied ──────────────────────────────────
		{
			"combined_string_bounds", "min=2,max=100", strType,
			Schema{MinLength: ptr(2), MaxLength: ptr(100)},
		},
		{
			"combined_numeric_bounds", "gte=1,lte=10", floatType,
			Schema{Minimum: ptr(1.0), Maximum: ptr(10.0)},
		},
		{
			"combined_format_and_bound", "email,max=254", strType,
			Schema{Format: "email", MaxLength: ptr(254)},
		},

		// ── tags that carry no OpenAPI meaning are silently ignored ───────────
		{"required_ignored", "required", strType, Schema{}},
		{"omitempty_ignored", "omitempty", strType, Schema{}},
		{"empty_tag", "", strType, Schema{}},
		{"unknown_tag", "foobar", strType, Schema{}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &Schema{}
			applyValidate(s, tc.tag, tc.typ)
			if diff := cmp.Diff(tc.want, *s); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestSchemaOf covers the non-obvious type-mapping edge cases in schemaOf that are
// not already exercised by TestBuildSpec: time.Time, []byte, double-pointer dereference,
// and unsigned integers mapping to "integer" rather than "number".
func TestSchemaOf(t *testing.T) {
	tests := []struct {
		name string
		typ  reflect.Type
		want Schema
	}{
		// time.Time is intercepted before the kind switch.
		{"time_time", reflect.TypeOf(time.Time{}), Schema{Type: "string", Format: "date-time"}},

		// []byte is a []uint8 slice but maps to base64 string, not array.
		{"bytes_base64", reflect.TypeOf([]byte{}), Schema{Type: "string", Format: "byte"}},

		// uint variants must map to "integer", not "number".
		{"uint", reflect.TypeOf(uint(0)), Schema{Type: "integer"}},
		{"uint64", reflect.TypeOf(uint64(0)), Schema{Type: "integer"}},

		// Multiple levels of pointer indirection are all stripped.
		{"ptr_ptr_string", reflect.TypeOf((**string)(nil)), Schema{Type: "string"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := newCollector().schemaOf(tc.typ)
			if diff := cmp.Diff(tc.want, *got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
