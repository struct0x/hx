package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/struct0x/hx"
	"github.com/struct0x/hx/hxtest"
)

type OrderServiceMock func(ctx context.Context, userID, product string, quantity int) (*Order, error)

func (o OrderServiceMock) CreateOrder(ctx context.Context, userID, product string, quantity int) (*Order, error) {
	return o(ctx, userID, product, quantity)
}

func TestOrderHandler(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(t *testing.T) (*http.Request, hx.HandlerFunc)
		checks []hxtest.Check
	}{
		{
			name: "method_not_allowed",
			setup: func(t *testing.T) (*http.Request, hx.HandlerFunc) {
				mock := OrderServiceMock(func(ctx context.Context, userID, product string, quantity int) (*Order, error) {
					t.Fatalf("unexpected call to OrderServiceMock")
					return nil, nil
				})

				req := httptest.NewRequest(http.MethodGet, "/orders", nil)

				return req, HandleCreateOrder(mock)
			},
			checks: []hxtest.Check{
				hxtest.IsProblem(
					hx.MethodNotAllowed(
						"Method GET not allowed",
						hx.WithDetail("Only POST method is allowed"),
						hx.WithTypeURI("https://example.com/schemas/create-order"),
					),
				),
			},
		},
		{
			name: "bad_request",
			setup: func(t *testing.T) (*http.Request, hx.HandlerFunc) {
				mock := OrderServiceMock(func(ctx context.Context, userID, product string, quantity int) (*Order, error) {
					t.Fatalf("unexpected call to OrderServiceMock")
					return nil, nil
				})

				req := httptest.NewRequest(http.MethodPost, "/orders", nil)

				return req, HandleCreateOrder(mock)
			},
			checks: []hxtest.Check{},
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
