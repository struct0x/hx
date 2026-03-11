// Package hxtest provides a fluent API for testing hx.HandlerFunc.
package hxtest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/struct0x/hx"
	"github.com/struct0x/hx/internal"
	"github.com/struct0x/hx/internal/out"
)

// Result is a materialized HTTP response produced by running a handler.
type Result struct {
	ContentType string
	Status      int
	Header      http.Header

	Body     any
	Hijacked bool
	Err      error

	Problem *out.ProblemDetails
}

// Check is a test check applied to a Result.
type Check func(t testing.TB, r *Result)

// Tester is the harness builder. Create via Test(t, handler).
type Tester struct {
	t           testing.TB
	handler     hx.HandlerFunc
	ctxMutators []func(context.Context) context.Context
	expects     []Check
	debugBody   bool
}

// Test initializes a test harness for a single handler.
func Test(t testing.TB, h hx.HandlerFunc) *Tester {
	t.Helper()
	return &Tester{t: t, handler: h}
}

// WithContext lets you mutate the request context before invocation.
func (tt *Tester) WithContext(mut func(ctx context.Context) context.Context) *Tester {
	tt.ctxMutators = append(tt.ctxMutators, mut)
	return tt
}

// Expect queues an assertion to run after the handler is executed.
func (tt *Tester) Expect(e Check) *Tester {
	tt.expects = append(tt.expects, e)
	return tt
}

// Expects queues multiple assertions to run after the handler is executed.
func (tt *Tester) Expects(ee ...Check) *Tester {
	tt.expects = append(tt.expects, ee...)
	return tt
}

// DebugBody enables debug logging of the response body.
func (tt *Tester) DebugBody(b bool) *Tester {
	tt.debugBody = b
	return tt
}

// Do run the handler against req, honoring HijackResponseWriter semantics,
// materializes the result, then runs queued expectations (fail-fast).
func (tt *Tester) Do(req *http.Request) *Result {
	tt.t.Helper()

	rec := httptest.NewRecorder()
	ctx := req.Context()
	for _, mut := range tt.ctxMutators {
		ctx = mut(ctx)
	}

	rwRead := &atomic.Bool{}
	ctx = internal.WithResponseWriter(ctx, rwRead, rec)
	req = req.WithContext(ctx)

	hxErr := tt.handler(ctx, req)

	var res *Result
	if rwRead.Load() {
		res = buildResultFromRecorder(rec)
		res.Hijacked = true
		res.Err = hxErr
	} else {
		res = materialize(tt.t, hxErr)
	}

	if tt.debugBody {
		tt.t.Logf("body:\n %s", pp(res.Body))
	}

	for _, e := range tt.expects {
		e(tt.t, res)
	}

	return res
}

func pp(d any) string {
	dd, _ := json.MarshalIndent(d, "", "\t")
	return string(dd)
}

// materialize applies hx rules.
func materialize(t testing.TB, hxErr error) *Result {
	t.Helper()

	var pd out.ProblemDetails
	if errors.As(hxErr, &pd) {
		return &Result{
			ContentType: "application/problem+json",
			Status:      pd.StatusCode,
			Header:      pd.Headers.Clone(),
			Body:        pd,
			Err:         hxErr,
			Problem:     &pd,
		}
	}

	var resp out.Response
	if errors.As(hxErr, &resp) {
		return &Result{
			ContentType: resp.ContentType,
			Status:      resp.StatusCode,
			Header:      resp.Headers.Clone(),
			Body:        resp.Body,
			Err:         hxErr,
		}
	}

	return &Result{
		ContentType: "application/json",
		Status:      http.StatusInternalServerError,
		Header:      nil,
		Body:        nil,
		Err:         hxErr,
	}
}

func buildResultFromRecorder(rec *httptest.ResponseRecorder) *Result {
	return &Result{
		Status: rec.Code,
		Header: rec.Header().Clone(),
		Body:   rec.Body.Bytes(),
	}
}

func Status(code int) Check {
	return func(t testing.TB, r *Result) {
		t.Helper()
		if r.Status != code {
			t.Errorf("status: got %d, want %d", r.Status, code)
		}
	}
}

func Header(key, want string) Check {
	return func(t testing.TB, r *Result) {
		t.Helper()
		got := r.Header.Get(key)
		if got != want {
			t.Errorf("header %q: got %q, want %q", key, got, want)
		}
	}
}

func HeaderHas(key, substr string) Check {
	return func(t testing.TB, r *Result) {
		t.Helper()
		got := r.Header.Get(key)
		if !strings.Contains(got, substr) {
			t.Errorf("header %q: %q does not contain %q", key, got, substr)
		}
	}
}

func NoBody() Check {
	return func(t testing.TB, r *Result) {
		t.Helper()
		if r.Body != nil {
			t.Errorf("expected no body, got %T: %#v", r.Body, r.Body)
		}
	}
}

func Body(want any, opts ...cmp.Option) Check {
	return func(t testing.TB, r *Result) {
		t.Helper()

		opts2 := make([]cmp.Option, len(opts), len(opts)+1)
		copy(opts2, opts)
		opts2 = append(opts2, cmp.AllowUnexported())

		if diff := cmp.Diff(want, r.Body); diff != "" {
			t.Errorf("Body() mismatch (-want +got):\n%s", diff)
		}
	}
}

// IsProblem asserts RFC 9457 style response (& Content-Type).
func IsProblem(problem error) Check {
	return func(t testing.TB, r *Result) {
		t.Helper()

		if r.Problem == nil {
			t.Fatalf("expected problem body, got none/invalid JSON")
		}

		if diff := cmp.Diff(problem, *r.Problem); diff != "" {
			t.Errorf("problem details mismatch (-want +got):\n%s", diff)
		}
	}
}

func ProblemTitle(substr string) Check {
	return func(t testing.TB, r *Result) {
		t.Helper()
		if r.Problem == nil || !strings.Contains(r.Problem.Title, substr) {
			t.Fatalf("problem title does not contain %q; got %q", substr, r.Problem.Title)
		}
	}
}

func ProblemDetail(substr string) Check {
	return func(t testing.TB, r *Result) {
		t.Helper()
		if r.Problem == nil || !strings.Contains(r.Problem.Detail, substr) {
			t.Fatalf("problem detail does not contain %q; got %q", substr, r.Problem.Detail)
		}
	}
}
