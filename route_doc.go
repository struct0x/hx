package hx

import (
	"strings"
)

// Documented is implemented by handler types that self-describe their API contract.
// When a handler passed to Handle implements Documented, hx extracts the RouteDoc
// automatically — no separate hx.Doc call is needed at the registration site.
//
// This is the preferred pattern for production handlers: the documentation lives
// alongside the handler logic and travels with it through refactors.
//
// Example:
//
//    type CreateOrderHandler struct {
//        svc OrderService
//    }
//
//    func (h CreateOrderHandler) ServeHX(ctx context.Context, r *http.Request) error {
//        // handler logic
//    }
//
//    func (h CreateOrderHandler) Doc() hx.RouteDoc {
//        return hx.RouteDoc{
//            Summary:  "Create a new order",
//            Tags:     []string{"orders"},
//            Request:  CreateOrderRequest{},
//            Responses: map[int]any{
//                http.StatusCreated: CreateOrderResponse{},
//            },
//            Errors: []int{http.StatusUnprocessableEntity, http.StatusUnauthorized},
//        }
//    }
//
//    server.Handle("POST /orders", CreateOrderHandler{svc}, authMiddleware)

type Documented interface {
	Doc() RouteDoc
}

// RouteOpt is implemented by Middleware and Doc.
// Pass one or more RouteOpts to HandleFunc to configure a route.
type RouteOpt interface {
	routeOpt()
}

// RouteDoc describes an HTTP operation for API documentation and spec generation.
type RouteDoc struct {
	// Request is a zero-value of the request struct.
	// Struct tags drive parameter and body extraction:
	//   json:     request body field
	//   path:     path parameter
	//   query:    query parameter
	//   header:   header parameter
	//   cookie:   cookie parameter
	//   validate: constraints (required, min, max, enum, ...)
	Request any

	// Response is a zero-value of the primary success response body (200 OK).
	Response any

	// Responses document additional status codes and their response body types.
	// Takes precedence over Response for the same status code.
	// Use nil as the value to document a no-body response (e.g. 204).
	//   map[int]any{
	//       http.StatusCreated:   OrderResponse{},
	//       http.StatusNoContent: nil,
	//   }
	Responses map[int]any

	// Errors list extra HTTP status codes this route returns as ProblemDetails.
	// 400 and 500 are implicitly included on all routes.
	// Only declare route-specific ones: 404, 409, 422, etc.
	Errors []int

	// Tags group this operation in the generated spec (e.g. "orders", "users").
	Tags []string

	// Summary is a short one-line description shown in tooling.
	Summary string

	// Description is a longer explanation. Markdown supported.
	Description string

	// OperationID is a unique identifier for code generators.
	// Auto-derived from method + path if empty (e.g. "post_orders").
	OperationID string

	// Security references security scheme names defined at the server level.
	// e.g. []string{"BearerAuth"}
	Security []string

	// Deprecated marks this operation as deprecated in the spec.
	Deprecated bool
}

// docOpt wraps RouteDoc to implement RouteOpt.
type docOpt struct {
	d RouteDoc
}

func (docOpt) routeOpt() {}

// Doc wraps a RouteDoc to be passed to HandleFunc alongside middlewares.
//
//	server.HandleFunc("POST /orders", HandleCreateOrder(svc),
//	    hx.Doc(hx.RouteDoc{
//	        Request:  CreateOrderRequest{},
//	        Response: OrderResponse{},
//	        Summary:  "Create a new order",
//	        Tags:     []string{"orders"},
//	    }),
//	)
func Doc(d RouteDoc) RouteOpt {
	return docOpt{d}
}

// RouteInfo is a registered route with its method, path, and optional documentation.
type RouteInfo struct {
	Method string
	Path   string
	Doc    *RouteDoc
}

// Routes returns all routes registered on this server, including those registered via groups.
func (h *HX) Routes() []RouteInfo {
	return *h.routes
}

// splitPattern splits a Go 1.22+ pattern like "POST /orders" into ("POST", "/orders").
// Returns ("*", pattern) for patterns without a method prefix (e.g. "/orders").
func splitPattern(pattern string) (method, path string) {
	if i := strings.Index(pattern, " "); i != -1 {
		return pattern[:i], pattern[i+1:]
	}
	return "*", pattern
}
