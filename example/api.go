package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/struct0x/hx"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("pong"))
	})

	svc := &OrderService{}

	server := hx.New(
		hx.WithCustomMux(mux),
		hx.WithMiddlewares(loggingMiddleware),
		hx.WithProblemInstanceGetter(func(ctx context.Context) string {
			// In a real app, return a trace/request ID from ctx.
			return "trace-id"
		}),
	)

	// All /api/v1 routes require authentication.
	api := server.Group("/api/v1", authMiddleware)
	api.Handle("POST /orders", HandleCreateOrder(svc))
	api.Handle("GET /orders", HandleListOrders(svc))
	api.Handle("GET /orders/{id}", HandleGetOrder(svc))

	// Admin routes require an additional privilege check.
	admin := server.Group("/admin", authMiddleware, adminMiddleware)
	admin.Handle("GET /stats", HandleGetStats(svc))

	fmt.Println("Server starting on :8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		fmt.Printf("Server error: %v\n", err)
	}
}

// loggingMiddleware logs every request method, path, and any error returned.
func loggingMiddleware(next hx.HandlerFunc) hx.HandlerFunc {
	return func(ctx context.Context, r *http.Request) error {
		err := next(ctx, r)
		slog.Info("request", "method", r.Method, "path", r.URL.Path, "err", err)
		return err
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

// adminMiddleware restricts access to requests carrying a specific role header.
func adminMiddleware(next hx.HandlerFunc) hx.HandlerFunc {
	return func(ctx context.Context, r *http.Request) error {
		if r.Header.Get("X-Role") != "admin" {
			return hx.Forbidden("admin role required")
		}
		return next(ctx, r)
	}
}

// --- Interfaces ---

type iOrderService interface {
	CreateOrder(ctx context.Context, userID, product string, quantity int) (*Order, error)
	GetOrder(ctx context.Context, id string) (*Order, error)
	GetUserOrders(ctx context.Context, userID string, status, sortBy string, limit int) ([]Order, error)
	Stats(ctx context.Context) (*StatsResponse, error)
}

// --- Handlers ---

func HandleCreateOrder(svc iOrderService) hx.HandlerFunc {
	type request struct {
		UserID      string `header:"X-User-ID"  validate:"required"`
		ProductName string `json:"product_name" validate:"required"`
		Quantity    int    `json:"quantity"     validate:"required,gte=1,lte=1000"`
	}

	type response struct {
		Order   *Order `json:"order"`
		Message string `json:"message"`
	}

	return func(ctx context.Context, r *http.Request) error {
		var req request
		if err := hx.Bind(r, &req); err != nil {
			return hx.BindProblem(err, "invalid request")
		}

		order, err := svc.CreateOrder(ctx, req.UserID, req.ProductName, req.Quantity)
		if err != nil {
			return hx.Problem(http.StatusInternalServerError, "failed to create order",
				hx.WithDetail(err.Error()))
		}

		return hx.Created(response{Order: order, Message: "order created"})
	}
}

func HandleListOrders(svc iOrderService) hx.HandlerFunc {
	type request struct {
		UserID string `header:"X-User-ID" validate:"required"`
		Status string `query:"status"`
		SortBy string `query:"sort_by"`
		Limit  string `query:"limit"`
	}

	return func(ctx context.Context, r *http.Request) error {
		var req request
		if err := hx.Bind(r, &req); err != nil {
			return hx.BindProblem(err, "invalid request")
		}

		limit, _ := strconv.Atoi(req.Limit)

		orders, err := svc.GetUserOrders(ctx, req.UserID, req.Status, req.SortBy, limit)
		if err != nil {
			return hx.Problem(http.StatusInternalServerError, "failed to list orders",
				hx.WithDetail(err.Error()))
		}

		return hx.OK(orders)
	}
}

func HandleGetOrder(svc iOrderService) hx.HandlerFunc {
	return func(ctx context.Context, r *http.Request) error {
		id := r.PathValue("id")

		order, err := svc.GetOrder(ctx, id)
		if err != nil {
			return hx.NotFound("order not found",
				hx.WithDetail(fmt.Sprintf("order %q does not exist", id)))
		}

		return hx.OK(order)
	}
}

func HandleGetStats(svc iOrderService) hx.HandlerFunc {
	return func(ctx context.Context, r *http.Request) error {
		stats, err := svc.Stats(ctx)
		if err != nil {
			return hx.Problem(http.StatusInternalServerError, "failed to fetch stats",
				hx.WithDetail(err.Error()))
		}

		return hx.OK(stats)
	}
}

// --- Domain ---

type Order struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	ProductName string    `json:"product_name"`
	Quantity    int       `json:"quantity"`
	TotalPrice  float64   `json:"total_price"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

type StatsResponse struct {
	TotalOrders   int     `json:"total_orders"`
	TotalRevenue  float64 `json:"total_revenue"`
	PendingOrders int     `json:"pending_orders"`
}

// --- OrderService ---

type OrderService struct{}

func (s *OrderService) CreateOrder(ctx context.Context, userID, product string, quantity int) (*Order, error) {
	if quantity <= 0 {
		return nil, fmt.Errorf("invalid quantity")
	}

	return &Order{
		ID:          fmt.Sprintf("ORD-%d", time.Now().Unix()),
		UserID:      userID,
		ProductName: product,
		Quantity:    quantity,
		TotalPrice:  float64(quantity) * 29.99,
		Status:      "pending",
		CreatedAt:   time.Now(),
	}, nil
}

func (s *OrderService) GetOrder(ctx context.Context, id string) (*Order, error) {
	if id != "ORD-001" {
		return nil, fmt.Errorf("not found")
	}

	return &Order{
		ID:          "ORD-001",
		ProductName: "Widget Pro",
		Quantity:    2,
		TotalPrice:  59.98,
		Status:      "completed",
		CreatedAt:   time.Now().Add(-48 * time.Hour),
	}, nil
}

func (s *OrderService) GetUserOrders(ctx context.Context, userID string, status, sortBy string, limit int) ([]Order, error) {
	orders := []Order{
		{ID: "ORD-001", UserID: userID, ProductName: "Widget Pro", Quantity: 2, TotalPrice: 59.98, Status: "completed", CreatedAt: time.Now().Add(-48 * time.Hour)},
		{ID: "ORD-002", UserID: userID, ProductName: "Gadget Plus", Quantity: 1, TotalPrice: 149.99, Status: "pending", CreatedAt: time.Now().Add(-24 * time.Hour)},
	}

	if status != "" {
		var filtered []Order
		for _, o := range orders {
			if o.Status == status {
				filtered = append(filtered, o)
			}
		}
		orders = filtered
	}

	if limit > 0 && limit < len(orders) {
		orders = orders[:limit]
	}

	return orders, nil
}

func (s *OrderService) Stats(ctx context.Context) (*StatsResponse, error) {
	return &StatsResponse{
		TotalOrders:   2,
		TotalRevenue:  209.97,
		PendingOrders: 1,
	}, nil
}