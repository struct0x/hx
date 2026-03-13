package hxdoc

import (
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Schema is a JSON Schema definition for OpenAPI 3.1.0.
type Schema struct {
	Ref                  string             `json:"$ref,omitempty"`
	Type                 string             `json:"type,omitempty"`
	Format               string             `json:"format,omitempty"`
	Pattern              string             `json:"pattern,omitempty"`
	Properties           map[string]*Schema `json:"properties,omitempty"`
	Required             []string           `json:"required,omitempty"`
	Items                *Schema            `json:"items,omitempty"`
	AdditionalProperties *Schema            `json:"additionalProperties,omitempty"`
	Enum                 []any              `json:"enum,omitempty"`
	Minimum              *float64           `json:"minimum,omitempty"`
	Maximum              *float64           `json:"maximum,omitempty"`
	ExclusiveMinimum     *float64           `json:"exclusiveMinimum,omitempty"`
	ExclusiveMaximum     *float64           `json:"exclusiveMaximum,omitempty"`
	MinLength            *int               `json:"minLength,omitempty"`
	MaxLength            *int               `json:"maxLength,omitempty"`
	MinItems             *int               `json:"minItems,omitempty"`
	MaxItems             *int               `json:"maxItems,omitempty"`
	UniqueItems          bool               `json:"uniqueItems,omitempty"`
}

var timeType = reflect.TypeOf(time.Time{})

// collector builds JSON schemas and registers named struct types into components/schemas,
// replacing their inline definitions with $ref pointers.
type collector struct {
	schemas map[string]*Schema
}

func newCollector() *collector {
	return &collector{schemas: make(map[string]*Schema)}
}

// schemaOf returns the Schema for t. Named struct types are registered in c.schemas
// and referenced via $ref so they appear in the Swagger UI Schemas panel.
func (c *collector) schemaOf(t reflect.Type) *Schema {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if t == timeType {
		return &Schema{Type: "string", Format: "date-time"}
	}

	switch t.Kind() {
	case reflect.String:
		return &Schema{Type: "string"}
	case reflect.Bool:
		return &Schema{Type: "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &Schema{Type: "integer"}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &Schema{Type: "integer"}
	case reflect.Float32, reflect.Float64:
		return &Schema{Type: "number"}
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 { // []byte → base64 string
			return &Schema{Type: "string", Format: "byte"}
		}
		return &Schema{Type: "array", Items: c.schemaOf(t.Elem())}
	case reflect.Map:
		return &Schema{Type: "object", AdditionalProperties: c.schemaOf(t.Elem())}
	case reflect.Struct:
		return c.structRef(t)
	default:
		return &Schema{}
	}
}

// structRef registers t in components/schemas (if named) and returns a $ref or inline schema.
func (c *collector) structRef(t reflect.Type) *Schema {
	name := t.Name()
	if name == "" {
		return c.buildStructSchema(t)
	}

	if _, exists := c.schemas[name]; !exists {
		c.schemas[name] = &Schema{}
		c.schemas[name] = c.buildStructSchema(t)
	}

	return &Schema{Ref: "#/components/schemas/" + name}
}

// buildStructSchema builds a full object schema from all json-tagged fields of t.
func (c *collector) buildStructSchema(t reflect.Type) *Schema {
	s := &Schema{
		Type:       "object",
		Properties: make(map[string]*Schema),
	}

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}

		jsonTag := f.Tag.Get("json")
		if jsonTag == "-" {
			continue
		}

		name := fieldName(jsonTag, f.Name)
		fs := c.schemaOf(f.Type)
		applyValidate(fs, f.Tag.Get("validate"), f.Type)

		if strings.Contains(f.Tag.Get("validate"), "required") {
			s.Required = append(s.Required, name)
		}

		s.Properties[name] = fs
	}

	return s
}

// extractParams returns OpenAPI parameters from path/query/header/cookie struct tags.
func (c *collector) extractParams(t reflect.Type) []parameter {
	var params []parameter

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}

		for _, loc := range []string{"path", "query", "header", "cookie"} {
			tag := f.Tag.Get(loc)
			if tag == "" || tag == "-" {
				continue
			}
			name := strings.SplitN(tag, ",", 2)[0]
			fs := c.schemaOf(f.Type)
			applyValidate(fs, f.Tag.Get("validate"), f.Type)

			params = append(params, parameter{
				Name:     name,
				In:       loc,
				Required: loc == "path" || strings.Contains(f.Tag.Get("validate"), "required"),
				Schema:   fs,
			})
		}
	}

	return params
}

// extractBodySchema returns an inline schema built from the json-tagged, non-parameter fields of t.
// Returns nil if the struct has no body fields.
// Field types are resolved through the collector so nested named structs get registered.
func (c *collector) extractBodySchema(t reflect.Type) *Schema {
	s := &Schema{
		Type:       "object",
		Properties: make(map[string]*Schema),
	}

	hasFields := false

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}

		jsonTag := f.Tag.Get("json")
		if jsonTag == "" || jsonTag == "-" {
			continue
		}

		if f.Tag.Get("path") != "" || f.Tag.Get("query") != "" ||
			f.Tag.Get("header") != "" || f.Tag.Get("cookie") != "" {
			continue
		}

		name := fieldName(jsonTag, f.Name)
		fs := c.schemaOf(f.Type)
		applyValidate(fs, f.Tag.Get("validate"), f.Type)

		if strings.Contains(f.Tag.Get("validate"), "required") {
			s.Required = append(s.Required, name)
		}

		s.Properties[name] = fs
		hasFields = true
	}

	if !hasFields {
		return nil
	}

	return s
}

func fieldName(tag, fallback string) string {
	name := strings.SplitN(tag, ",", 2)[0]
	if name == "" {
		return fallback
	}
	return name
}

func applyValidate(s *Schema, tag string, t reflect.Type) {
	if tag == "" {
		return
	}

	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	for _, part := range strings.Split(tag, ",") {
		part = strings.TrimSpace(part)
		switch {

		case part == "email":
			s.Format = "email"
		case part == "url", part == "uri":
			s.Format = "uri"
		case part == "uuid", part == "uuid3", part == "uuid4", part == "uuid5":
			s.Format = "uuid"
		case part == "hostname", part == "hostname_rfc1123", part == "fqdn":
			s.Format = "hostname"
		case part == "ip", part == "ip_addr":
			s.Format = "ip"
		case part == "ipv4", part == "ip4_addr":
			s.Format = "ipv4"
		case part == "ipv6", part == "ip6_addr":
			s.Format = "ipv6"
		case part == "cidr", part == "cidrv4", part == "cidrv6":
			s.Format = "cidr"
		case part == "mac":
			s.Format = "mac-address"
		case part == "base64", part == "base64url", part == "base64rawurl":
			s.Format = "byte"
		case part == "base32":
			s.Format = "base32"
		case part == "jwt":
			s.Format = "jwt"
		case part == "semver":
			s.Format = "semver"
		case part == "credit_card":
			s.Format = "credit-card"
		case part == "iso3166_1_alpha2":
			s.Format = "iso3166-1-alpha-2"
		case part == "iso3166_1_alpha3":
			s.Format = "iso3166-1-alpha-3"
		case part == "bcp47_language_tag":
			s.Format = "bcp47-language-tag"
		case part == "timezone":
			s.Format = "timezone"
		case strings.HasPrefix(part, "datetime="):
			s.Format = "date-time"

		case part == "alpha":
			s.Pattern = "^[a-zA-Z]+$"
		case part == "alphaspace":
			s.Pattern = "^[a-zA-Z ]+$"
		case part == "alphanum":
			s.Pattern = "^[a-zA-Z0-9]+$"
		case part == "numeric":
			s.Pattern = "^[0-9]+$"
		case part == "number":
			s.Pattern = "^[-+]?[0-9]*\\.?[0-9]+$"
		case part == "hexadecimal":
			s.Pattern = "^(0[xX])?[0-9a-fA-F]+$"
		case part == "hexcolor":
			s.Pattern = "^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$"
		case part == "e164":
			s.Pattern = `^\+[1-9]\d{1,14}$`

		case strings.HasPrefix(part, "gte="):
			v := parseFloat(part[4:])
			s.Minimum = &v
		case strings.HasPrefix(part, "lte="):
			v := parseFloat(part[4:])
			s.Maximum = &v
		case strings.HasPrefix(part, "gt="):
			v := parseFloat(part[3:])
			s.ExclusiveMinimum = &v
		case strings.HasPrefix(part, "lt="):
			v := parseFloat(part[3:])
			s.ExclusiveMaximum = &v
		case strings.HasPrefix(part, "min="):
			n := parseInt(part[4:])
			switch t.Kind() {
			case reflect.String:
				s.MinLength = &n
			case reflect.Slice, reflect.Array:
				s.MinItems = &n
			default:
				v := float64(n)
				s.Minimum = &v
			}
		case strings.HasPrefix(part, "max="):
			n := parseInt(part[4:])
			switch t.Kind() {
			case reflect.String:
				s.MaxLength = &n
			case reflect.Slice, reflect.Array:
				s.MaxItems = &n
			default:
				v := float64(n)
				s.Maximum = &v
			}
		case strings.HasPrefix(part, "len="):
			n := parseInt(part[4:])
			s.MinLength = &n
			s.MaxLength = &n
		case part == "port":
			mn, mx := float64(1), float64(65535)
			s.Minimum = &mn
			s.Maximum = &mx
		case part == "latitude":
			mn, mx := -90.0, 90.0
			s.Minimum = &mn
			s.Maximum = &mx
		case part == "longitude":
			mn, mx := -180.0, 180.0
			s.Minimum = &mn
			s.Maximum = &mx

		case strings.HasPrefix(part, "oneof="), strings.HasPrefix(part, "oneofci="):
			raw := part[strings.Index(part, "=")+1:]
			for _, e := range strings.Fields(raw) {
				s.Enum = append(s.Enum, enumVal(e, t.Kind()))
			}
		case strings.HasPrefix(part, "eq="):
			s.Enum = []any{enumVal(part[3:], t.Kind())}

		case part == "unique":
			if t.Kind() == reflect.Slice || t.Kind() == reflect.Array || t.Kind() == reflect.Map {
				s.UniqueItems = true
			}
		}
	}
}

// enumVal converts a string value to the appropriate Go type based on reflect.Kind.
func enumVal(s string, k reflect.Kind) any {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if v, err := strconv.ParseInt(s, 10, 64); err == nil {
			return v
		}
	case reflect.Float32, reflect.Float64:
		if v, err := strconv.ParseFloat(s, 64); err == nil {
			return v
		}
	}
	return s
}

func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func parseInt(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}
