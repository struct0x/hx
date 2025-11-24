package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/struct0x/hx"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("pong"))
	})

	hmux := hx.New(
		hx.WithCustomMux(mux),
		hx.WithMiddlewares(),
	)

	hmux.Handle("/orders", HandleCreateOrder(&OrderService{}))

	fmt.Println("Server starting on :8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		fmt.Printf("Server error: %v\n", err)
	}
}

type iOrderService interface {
	CreateOrder(ctx context.Context, userID, product string, quantity int) (*Order, error)
}

// HandleCreateOrder demonstrates a semi-complex handler with:
// - Path parameters
// - Query parameters with defaults
// - Multiple HTTP methods (GET, POST)
// - Request validation
// - Service layer interaction
// - Structured responses
func HandleCreateOrder(orderService iOrderService) hx.HandlerFunc {
	type CreateOrderRequest struct {
		UserID string `header:"X-User-ID" validate:"required"`

		ProductName string `json:"product_name" validate:"required"`
		Quantity    int    `json:"quantity" validate:"required,gte=1,lte=1000"`
	}

	type OrderResponse struct {
		Order   *Order  `json:"order,omitempty"`
		Orders  []Order `json:"orders,omitempty"`
		Message string  `json:"message,omitempty"`
	}

	return func(ctx context.Context, r *http.Request) error {
		if r.Method != http.MethodPost {
			return hx.MethodNotAllowed(
				fmt.Sprintf("Method %s not allowed", r.Method),
				hx.WithDetail("Only POST method is allowed"),
				hx.WithTypeURI("https://example.com/schemas/create-order"),
			)
		}

		var req CreateOrderRequest
		if err := hx.Bind(r, &req); err != nil {
			return hx.BindProblem(
				err,
				"invalid request",
				hx.WithTypeURI("https://example.com/schemas/create-order"),
			)
		}

		// Create order through service
		order, err := orderService.CreateOrder(ctx, req.UserID, req.ProductName, req.Quantity)
		if err != nil {
			return hx.Problem(
				http.StatusInternalServerError,
				"failed to create order",
				hx.WithDetail(err.Error()),
			)
		}

		return hx.Created(OrderResponse{
			Order:   order,
			Message: "Order created successfully",
		})
	}
}

// OrderService simulates a service layer
type OrderService struct{}

type Order struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	ProductName string    `json:"product_name"`
	Quantity    int       `json:"quantity"`
	TotalPrice  float64   `json:"total_price"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

func (s *OrderService) CreateOrder(ctx context.Context, userID, product string, quantity int) (*Order, error) {
	// Simulate business logic
	if quantity <= 0 {
		return nil, fmt.Errorf("invalid quantity")
	}

	pricePerUnit := 29.99
	order := &Order{
		ID:          fmt.Sprintf("ORD-%d", time.Now().Unix()),
		UserID:      userID,
		ProductName: product,
		Quantity:    quantity,
		TotalPrice:  float64(quantity) * pricePerUnit,
		Status:      "pending",
		CreatedAt:   time.Now(),
	}
	return order, nil
}

func (s *OrderService) GetUserOrders(ctx context.Context, userID string, status, sortBy string, limit int) ([]Order, error) {
	// Simulate fetching orders from database
	orders := []Order{
		{
			ID:          "ORD-001",
			UserID:      userID,
			ProductName: "Widget Pro",
			Quantity:    2,
			TotalPrice:  59.98,
			Status:      "completed",
			CreatedAt:   time.Now().Add(-48 * time.Hour),
		},
		{
			ID:          "ORD-002",
			UserID:      userID,
			ProductName: "Gadget Plus",
			Quantity:    1,
			TotalPrice:  149.99,
			Status:      "pending",
			CreatedAt:   time.Now().Add(-24 * time.Hour),
		},
	}

	// Filter by status if provided
	if status != "" {
		var filtered []Order
		for _, order := range orders {
			if order.Status == status {
				filtered = append(filtered, order)
			}
		}
		orders = filtered
	}

	// Apply limit
	if limit > 0 && limit < len(orders) {
		orders = orders[:limit]
	}

	return orders, nil
}
