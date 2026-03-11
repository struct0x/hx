package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/struct0x/hx"
	"github.com/struct0x/hx/hxtest"
)

// OrderServiceMock implements iOrderService for testing.
type OrderServiceMock struct {
	createOrder func(ctx context.Context, userID, product string, quantity int) (*Order, error)
}

func (m *OrderServiceMock) CreateOrder(ctx context.Context, userID, product string, quantity int) (*Order, error) {
	return m.createOrder(ctx, userID, product, quantity)
}

func (m *OrderServiceMock) GetOrder(_ context.Context, _ string) (*Order, error) { return nil, nil }
func (m *OrderServiceMock) GetUserOrders(_ context.Context, _, _, _ string, _ int) ([]Order, error) {
	return nil, nil
}
func (m *OrderServiceMock) Stats(_ context.Context) (*StatsResponse, error) { return nil, nil }

func TestHandleCreateOrder(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(t *testing.T) (*http.Request, hx.HandlerFunc)
		checks []hxtest.Check
	}{
		{
			name: "bad_request_missing_body",
			setup: func(t *testing.T) (*http.Request, hx.HandlerFunc) {
				mock := &OrderServiceMock{
					createOrder: func(ctx context.Context, userID, product string, quantity int) (*Order, error) {
						t.Fatalf("unexpected call to CreateOrder")
						return nil, nil
					},
				}

				req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", nil)
				req.Header.Set("X-User-ID", "user-123")

				return req, HandleCreateOrder(mock)
			},
			checks: []hxtest.Check{
				hxtest.Status(http.StatusBadRequest),
			},
		},
		{
			name: "bad_request_missing_user_id",
			setup: func(t *testing.T) (*http.Request, hx.HandlerFunc) {
				mock := &OrderServiceMock{
					createOrder: func(ctx context.Context, userID, product string, quantity int) (*Order, error) {
						t.Fatalf("unexpected call to CreateOrder")
						return nil, nil
					},
				}

				body := strings.NewReader(`{"product_name":"Widget","quantity":2}`)
				req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", body)
				req.Header.Set("Content-Type", "application/json")

				return req, HandleCreateOrder(mock)
			},
			checks: []hxtest.Check{
				hxtest.Status(http.StatusBadRequest),
			},
		},
		{
			name: "created",
			setup: func(t *testing.T) (*http.Request, hx.HandlerFunc) {
				mock := &OrderServiceMock{
					createOrder: func(_ context.Context, userID, product string, quantity int) (*Order, error) {
						return &Order{
							ID:          "ORD-999",
							UserID:      userID,
							ProductName: product,
							Quantity:    quantity,
							TotalPrice:  59.98,
							Status:      "pending",
						}, nil
					},
				}

				body := strings.NewReader(`{"product_name":"Widget","quantity":2}`)
				req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", body)
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-User-ID", "user-123")

				return req, HandleCreateOrder(mock)
			},
			checks: []hxtest.Check{
				hxtest.Status(http.StatusCreated),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, handler := tc.setup(t)

			hxtest.Test(t, handler).
				DebugBody(true).
				Expects(tc.checks...).
				Do(req)
		})
	}
}
