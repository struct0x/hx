// Package hx provides a lightweight HTTP framework for building RESTful APIs in Go.
// It focuses on simplifying request handling, response generation, and error management
// while maintaining compatibility with the standard library's http.Handler interface.
//
// The framework implements RFC 9457 (Problem Details for HTTP APIs) for standardized
// error responses and provides a comprehensive request binding system that extracts
// data from multiple sources including query parameters, path variables, headers,
// cookies, JSON bodies, form data, and file uploads.
//
// # Basic Usage
//
// Create a new HX instance and register handlers:
//
//	hx := hx.New()
//	hx.Handle("/users", func(ctx context.Context, r *http.Request) error {
//		// Handle the request
//		return hx.OK(map[string]string{"message": "success"})
//	})
//	http.ListenAndServe(":8080", hx)
//
// # Request Binding
//
// Extract request data into structs using struct tags:
//
//	type UserRequest struct {
//		ID       int    `path:"id"`
//		Name     string `json:"name" validate:"required"`
//		Email    string `json:"email" validate:"required,email"`
//		Auth     string `header:"Authorization"`
//		Tags     []string `query:"tags"`
//	}
//
//	func handler(ctx context.Context, r *http.Request) error {
//		var req UserRequest
//		if err := hx.Bind(r, &req); err != nil {
//			return hx.BindProblem(err, "Invalid request")
//		}
//		// Use req...
//		return nil
//	}
//
// # Error Handling
//
// Return structured errors using ProblemDetails:
//
//	func handler(ctx context.Context, r *http.Request) error {
//		if !authorized(r) {
//			return hx.Unauthorized("Access denied",
//				hx.WithDetail("Invalid authentication token"),
//				hx.WithTypeURI("https://example.com/errors/auth"))
//		}
//		return nil
//	}
//
// # Middleware
//
// Apply middleware to all handlers:
//
//	hx := hx.New(
//		hx.WithMiddlewares(loggingMiddleware, authMiddleware),
//		hx.WithLogger(logger),
//	)
//
// # Testing
//
// Test handlers using the hxtest package:
//
//	func TestHandler(t *testing.T) {
//		hxtest.Test(t, handler).
//			Do(httptest.NewRequest("GET", "/test", nil)).
//			Expect(hxtest.Status(http.StatusOK)).
//			Expect(hxtest.Body(map[string]string{"message": "success"}))
//	}
//
// # Response Types
//
// The framework supports multiple response types:
//   - hx.OK(body): 200 OK with JSON body
//   - hx.Created(body): 201 Created with JSON body
//   - hx.Problem(status, title): RFC 9457 problem details
//   - hx.Respond(status, body, opts...): Custom response with options
//
// # Configuration Options
//
// Configure the HX instance with functional options:
//   - WithLogger: Set a custom logger
//   - WithCustomMux: Use a custom http.Handler
//   - WithMiddlewares: Apply middleware functions
//   - WithProblemInstanceGetter: Set a function to generate problem instance URIs
//
// For more examples, see the example package.
package hx
