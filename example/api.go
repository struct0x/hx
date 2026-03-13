package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/struct0x/hx"
	"github.com/struct0x/hx/hxdoc"
	"github.com/struct0x/hx/hxmid"
)

func main() {
	server := hx.New(
		hx.WithCustomMux(http.NewServeMux()),
		hx.WithProductionMode(true),
		hx.WithMiddlewares(
			hxmid.Recoverer(slog.Default()),
			hxmid.Logger(slog.Default()),
			hxmid.RequireJSON(),
		),
		hx.WithProblemInstanceGetter(func(ctx context.Context) string {
			return "trace-id" // replace with real trace ID from ctx
		}),
	)

	// All /api/v1 routes require authentication.
	api := server.Group("/api/v1", authMiddleware)

	api.HandleFunc(
		"POST /orders",
		HandleCreateOrder(),
		hx.Doc(hx.RouteDoc{
			Summary:     "Create a new order",
			Description: "Creates an order for the authenticated user. Returns 201 with the new order.",
			Tags:        []string{"orders"},
			Security:    []string{"BearerAuth"},
			Request:     CreateOrderRequest{},
			Responses: map[int]any{
				http.StatusCreated: CreateOrderResponse{},
			},
			Errors: []int{http.StatusBadRequest, http.StatusUnprocessableEntity, http.StatusUnauthorized},
		}),
	)

	api.HandleFunc("GET /orders", HandleListOrders(),
		hx.Doc(hx.RouteDoc{
			Summary:  "List orders",
			Tags:     []string{"orders"},
			Security: []string{"BearerAuth"},
			Request:  ListOrdersRequest{},
			Response: OrderList{},
			Errors:   []int{http.StatusUnauthorized},
		}),
	)

	api.HandleFunc("GET /orders/{id}", HandleGetOrder(),
		hx.Doc(hx.RouteDoc{
			Summary:  "Get an order by ID",
			Tags:     []string{"orders"},
			Security: []string{"BearerAuth"},
			Request:  GetOrderRequest{},
			Response: Order{},
			Errors:   []int{http.StatusNotFound, http.StatusUnauthorized},
		}),
	)

	api.HandleFunc("DELETE /orders/{id}", HandleCancelOrder(),
		hxmid.RequireJSON(),
		hx.Doc(hx.RouteDoc{
			Summary:  "Cancel an order",
			Tags:     []string{"orders"},
			Security: []string{"BearerAuth"},
			Request:  CancelOrderRequest{},
			Responses: map[int]any{
				http.StatusNoContent: nil,
			},
			Errors: []int{http.StatusNotFound, http.StatusConflict, http.StatusUnauthorized},
		}),
	)

	api.HandleFunc("PATCH /orders/{id}/status", HandleUpdateOrderStatus(),
		hxmid.RequireJSON(),
		hx.Doc(hx.RouteDoc{
			Summary:  "Update order status",
			Tags:     []string{"orders"},
			Security: []string{"BearerAuth"},
			Request:  UpdateOrderStatusRequest{},
			Response: Order{},
			Errors:   []int{http.StatusNotFound, http.StatusConflict, http.StatusUnauthorized},
		}),
	)

	api.HandleFunc("POST /users", HandleCreateUser(),
		hxmid.RequireJSON(),
		hx.Doc(hx.RouteDoc{
			Summary:  "Register a new user",
			Tags:     []string{"users"},
			Security: []string{"BearerAuth"},
			Request:  CreateUserRequest{},
			Responses: map[int]any{
				http.StatusCreated: User{},
			},
			Errors: []int{http.StatusBadRequest, http.StatusConflict, http.StatusUnprocessableEntity},
		}),
	)

	api.HandleFunc("GET /users/{id}", HandleGetUser(),
		hx.Doc(hx.RouteDoc{
			Summary:  "Get a user by ID",
			Tags:     []string{"users"},
			Security: []string{"BearerAuth"},
			Request:  GetUserRequest{},
			Response: User{},
			Errors:   []int{http.StatusNotFound, http.StatusUnauthorized},
		}),
	)

	// Webhook routes are authenticated with a static API key via X-API-Key header.
	webhooks := server.Group("/webhooks", apiKeyMiddleware)

	webhooks.HandleFunc("POST /events", HandleIngestEvent(),
		hxmid.RequireJSON(),
		hx.Doc(hx.RouteDoc{
			Summary:  "Ingest an external event",
			Tags:     []string{"webhooks"},
			Security: []string{"ApiKeyAuth"},
			Request:  IngestEventRequest{},
			Responses: map[int]any{
				http.StatusAccepted: nil,
			},
			Errors: []int{http.StatusBadRequest, http.StatusUnauthorized},
		}),
	)

	// Admin routes require an additional privilege check.
	admin := server.Group("/admin", authMiddleware, adminMiddleware)

	admin.HandleFunc("GET /stats", HandleGetStats(),
		hx.Doc(hx.RouteDoc{
			Summary:  "Get platform statistics",
			Tags:     []string{"admin"},
			Security: []string{"BearerAuth"},
			Response: StatsResponse{},
			Errors:   []int{http.StatusUnauthorized, http.StatusForbidden},
		}),
	)

	admin.HandleFunc("POST /users/{id}/ban", HandleBanUser(),
		hxmid.RequireJSON(),
		hx.Doc(hx.RouteDoc{
			Summary:  "Ban a user",
			Tags:     []string{"admin"},
			Security: []string{"BearerAuth"},
			Request:  BanUserRequest{},
			Responses: map[int]any{
				http.StatusNoContent: nil,
			},
			Errors: []int{http.StatusNotFound, http.StatusConflict, http.StatusUnauthorized, http.StatusForbidden},
		}),
	)

	docs := server.Group("/docs")

	// OpenAPI spec and Swagger UI
	docs.HandleFunc("/openapi.json", hxdoc.Handler(server,
		hxdoc.WithTitle("Orders API"),
		hxdoc.WithVersion("1.0.0"),
		hxdoc.WithDescription("A simple order and user management API demonstrating hx framework capabilities."),
		hxdoc.WithServer("http://localhost:8080", "Local development"),
		hxdoc.WithSecurityScheme("BearerAuth", hxdoc.BearerAuth()),
		hxdoc.WithSecurityScheme("ApiKeyAuth", hxdoc.APIKeyAuth("header", "X-API-Key")),
	))
	docs.HandleFunc("/index.html", hxdoc.UIHandler("/docs/openapi.json"))

	fmt.Println("Server starting on :8080")
	fmt.Println("Swagger UI:   http://localhost:8080/docs/index.html")
	fmt.Println("OpenAPI JSON: http://localhost:8080/docs/openapi.json")

	printRoutes(server.Routes())

	if err := http.ListenAndServe(":8080", server); err != nil {
		fmt.Printf("Server error: %v\n", err)
	}
}

func printRoutes(routes []hx.RouteInfo) {
	for _, route := range routes {
		slog.Info("Route", "path", route.Path, "method", route.Method)
	}
}

// authMiddleware rejects requests without an Authorization header.
func authMiddleware(next hx.HandlerFunc) hx.HandlerFunc {
	return func(ctx context.Context, r *http.Request) error {
		if r.Header.Get("Authorization") == "" {
			return hx.Unauthorized("missing authorization header")
		}
		return next(ctx, r)
	}
}

// apiKeyMiddleware rejects requests without a valid X-API-Key header.
func apiKeyMiddleware(next hx.HandlerFunc) hx.HandlerFunc {
	const validKey = "secret-webhook-key" // in a real app, load from config
	return func(ctx context.Context, r *http.Request) error {
		if r.Header.Get("X-API-Key") != validKey {
			return hx.Unauthorized("invalid or missing API key")
		}
		return next(ctx, r)
	}
}

// adminMiddleware restricts access to requests carrying a specific role header.
func adminMiddleware(next hx.HandlerFunc) hx.HandlerFunc {
	return func(ctx context.Context, r *http.Request) error {
		if r.Header.Get("X-Role") != "admin" {
			return hx.Forbidden("admin role required")
		}
		return next(ctx, r)
	}
}

type CreateOrderRequest struct {
	UserID      string `header:"X-User-ID"  validate:"required"`
	ProductName string `json:"product_name" validate:"required,min=2,max=100"`
	Quantity    int    `json:"quantity"     validate:"required,gte=1,lte=1000"`
	Notes       string `json:"notes"        validate:"omitempty,max=500"`
}

type ListOrdersRequest struct {
	UserID  string `header:"X-User-ID" validate:"required"`
	Status  string `query:"status"     validate:"omitempty,oneof=pending processing shipped delivered cancelled"`
	SortBy  string `query:"sort_by"    validate:"omitempty,oneof=created_at total_price"`
	Page    int    `query:"page"       validate:"omitempty,gte=1"`
	PerPage int    `query:"per_page"   validate:"omitempty,gte=1,lte=100"`
}

type GetOrderRequest struct {
	ID string `path:"id" validate:"required"`
}

type CancelOrderRequest struct {
	ID     string `path:"id"     validate:"required"`
	Reason string `json:"reason" validate:"omitempty,max=500"`
}

type UpdateOrderStatusRequest struct {
	ID     string `path:"id"      validate:"required"`
	Status string `json:"status"  validate:"required,oneof=processing shipped delivered cancelled"`
}

type CreateUserRequest struct {
	Email     string `json:"email"      validate:"required,email"`
	FirstName string `json:"first_name" validate:"required,alpha,min=2,max=50"`
	LastName  string `json:"last_name"  validate:"required,alpha,min=2,max=50"`
	Phone     string `json:"phone"      validate:"omitempty,e164"`
	Country   string `json:"country"    validate:"omitempty,iso3166_1_alpha2"`
}

type GetUserRequest struct {
	ID      string `path:"id"      validate:"required"`
	Verbose bool   `query:"verbose"`
}

type BanUserRequest struct {
	ID     string `path:"id"      validate:"required"`
	Reason string `json:"reason"  validate:"required,min=10,max=500"`
}

type IngestEventRequest struct {
	EventType string         `json:"event_type" validate:"required,oneof=order.created order.cancelled user.registered"`
	Payload   map[string]any `json:"payload"    validate:"required"`
}

type CreateOrderResponse struct {
	Order   *Order `json:"order"`
	Message string `json:"message"`
}

type OrderList struct {
	Orders  []Order `json:"orders"`
	Total   int     `json:"total"`
	Page    int     `json:"page"`
	PerPage int     `json:"per_page"`
}

type Order struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	ProductName string    `json:"product_name"`
	Quantity    int       `json:"quantity"`
	TotalPrice  float64   `json:"total_price"`
	Status      string    `json:"status"`
	Notes       string    `json:"notes,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	FirstName string    `json:"first_name"`
	LastName  string    `json:"last_name"`
	Phone     string    `json:"phone,omitempty"`
	Country   string    `json:"country,omitempty"`
	Banned    bool      `json:"banned"`
	CreatedAt time.Time `json:"created_at"`
}

type StatsResponse struct {
	TotalOrders   int     `json:"total_orders"`
	TotalRevenue  float64 `json:"total_revenue"`
	PendingOrders int     `json:"pending_orders"`
	TotalUsers    int     `json:"total_users"`
}

func HandleCreateOrder() hx.HandlerFunc {
	return func(ctx context.Context, r *http.Request) error {
		var req CreateOrderRequest
		if err := hx.Bind(r, &req); err != nil {
			return hx.BindProblem(err, "invalid request")
		}

		return hx.Created(CreateOrderResponse{
			Order:   &Order{ID: "ORD-001", UserID: req.UserID, ProductName: req.ProductName, Quantity: req.Quantity, Status: "pending"},
			Message: "order created",
		})
	}
}

func HandleListOrders() hx.HandlerFunc {
	return func(ctx context.Context, r *http.Request) error {
		var req ListOrdersRequest
		if err := hx.Bind(r, &req); err != nil {
			return hx.BindProblem(err, "invalid request")
		}

		return hx.OK(&OrderList{
			Orders:  []Order{{ID: "ORD-001", UserID: req.UserID, ProductName: "Widget Pro", Status: "pending"}},
			Total:   1,
			Page:    req.Page,
			PerPage: req.PerPage,
		})
	}
}

func HandleGetOrder() hx.HandlerFunc {
	return func(ctx context.Context, r *http.Request) error {
		id := r.PathValue("id")
		if id == "" {
			return hx.NotFound("order not found")
		}

		return hx.OK(&Order{ID: id, ProductName: "Widget Pro", Status: "pending"})
	}
}

func HandleCancelOrder() hx.HandlerFunc {
	return func(ctx context.Context, r *http.Request) error {
		var req CancelOrderRequest
		if err := hx.Bind(r, &req); err != nil {
			return hx.BindProblem(err, "invalid request")
		}

		return hx.NoContent()
	}
}

func HandleUpdateOrderStatus() hx.HandlerFunc {
	return func(ctx context.Context, r *http.Request) error {
		var req UpdateOrderStatusRequest
		if err := hx.Bind(r, &req); err != nil {
			return hx.BindProblem(err, "invalid request")
		}

		return hx.OK(&Order{ID: req.ID, Status: req.Status})
	}
}

func HandleCreateUser() hx.HandlerFunc {
	return func(ctx context.Context, r *http.Request) error {
		var req CreateUserRequest
		if err := hx.Bind(r, &req); err != nil {
			return hx.BindProblem(err, "invalid request")
		}

		return hx.Created(&User{ID: "USR-001", Email: req.Email, FirstName: req.FirstName, LastName: req.LastName})
	}
}

func HandleGetUser() hx.HandlerFunc {
	return func(ctx context.Context, r *http.Request) error {
		var req GetUserRequest
		if err := hx.Bind(r, &req); err != nil {
			return hx.BindProblem(err, "invalid request")
		}

		if req.ID == "" {
			return hx.NotFound("user not found")
		}

		return hx.OK(&User{ID: req.ID, Email: "alice@example.com", FirstName: "Alice", LastName: "Smith"})
	}
}

func HandleIngestEvent() hx.HandlerFunc {
	return func(ctx context.Context, r *http.Request) error {
		var req IngestEventRequest
		if err := hx.Bind(r, &req); err != nil {
			return hx.BindProblem(err, "invalid request")
		}

		return hx.Respond(http.StatusAccepted, nil)
	}
}

func HandleGetStats() hx.HandlerFunc {
	return func(ctx context.Context, r *http.Request) error {
		return hx.OK(&StatsResponse{TotalOrders: 42, TotalRevenue: 3841.58, PendingOrders: 7, TotalUsers: 18})
	}
}

func HandleBanUser() hx.HandlerFunc {
	return func(ctx context.Context, r *http.Request) error {
		var req BanUserRequest
		if err := hx.Bind(r, &req); err != nil {
			return hx.BindProblem(err, "invalid request")
		}

		return hx.NoContent()
	}
}
