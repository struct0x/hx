package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/struct0x/hx"
	"github.com/struct0x/hx/hxtest"
)

func TestHandleCreateOrder(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(t *testing.T) (*http.Request, hx.HandlerFunc)
		checks []hxtest.Check
	}{
		{
			name: "bad_request_missing_body",
			setup: func(t *testing.T) (*http.Request, hx.HandlerFunc) {
				req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", nil)
				req.Header.Set("X-User-ID", "user-123")
				return req, HandleCreateOrder()
			},
			checks: []hxtest.Check{
				hxtest.Status(http.StatusBadRequest),
			},
		},
		{
			name: "bad_request_missing_user_id",
			setup: func(t *testing.T) (*http.Request, hx.HandlerFunc) {
				body := strings.NewReader(`{"product_name":"Widget","quantity":2}`)
				req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", body)
				req.Header.Set("Content-Type", "application/json")
				return req, HandleCreateOrder()
			},
			checks: []hxtest.Check{
				hxtest.Status(http.StatusBadRequest),
			},
		},
		{
			name: "created",
			setup: func(t *testing.T) (*http.Request, hx.HandlerFunc) {
				body := strings.NewReader(`{"product_name":"Widget","quantity":2}`)
				req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", body)
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-User-ID", "user-123")
				return req, HandleCreateOrder()
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
