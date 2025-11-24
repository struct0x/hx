package bind

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type benchQuery struct {
	Tags []string `query:"tags"`
}

type benchHeader struct {
	Agent string `header:"User-Agent"`
}

type benchCookie struct {
	Session string `cookie:"session_id"`
}

type benchPath struct {
	ID string `path:"id"`
}

type benchJSON struct {
	Name string `json:name"`
	Age  int    `json:"age"`
}

type benchFile struct {
	Avatar *multipart.FileHeader `file:"avatar"`
}

type benchForm struct {
	Title string `form:"title"`
	Count int    `form:"count"`
}

func BenchmarkBind_Query(b *testing.B) {
	values := url.Values{}
	// values.Add("tags", "go")
	req := httptest.NewRequest(http.MethodGet, "/?"+values.Encode(), nil)

	var dst benchQuery
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Reset destination to zero each iteration to avoid caching side‑effects
		dst = benchQuery{}
		Bind(req, &dst)
	}
}

func BenchmarkBind_Header(b *testing.B) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("User-Agent", "bench-agent/1.0")

	var dst benchHeader
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		dst = benchHeader{}
		if err := Bind(req, &dst); err != nil {
			b.Fatalf("bind error: %v", err)
		}
	}
}

func BenchmarkBind_Cookie(b *testing.B) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "benchmark-session"})

	var dst benchCookie
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		dst = benchCookie{}
		if err := Bind(req, &dst); err != nil {
			b.Fatalf("bind error: %v", err)
		}
	}
}

func BenchmarkBind_Path(b *testing.B) {
	req := httptest.NewRequest(http.MethodGet, "/users/12345", nil)

	// Simple PathValueFunc that splits on '/'.
	pathFn := func(r *http.Request, name string) string {
		if name != "id" {
			return ""
		}
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) >= 3 {
			return parts[2]
		}
		return ""
	}

	var dst benchPath
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		dst = benchPath{}
		if err := Bind(req, &dst, WithPathValueFunc(pathFn)); err != nil {
			b.Fatalf("bind error: %v", err)
		}
	}
}

func BenchmarkBind_JSON(b *testing.B) {
	payload := benchJSON{Name: "bench", Age: 99}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	var dst benchJSON
	b.ReportAllocs()
	b.ResetTimer()

	buf := bytes.NewReader(body)

	for i := 0; i < b.N; i++ {
		buf.Seek(0, io.SeekStart)
		dst = benchJSON{}

		if err := Bind(req, &dst); err != nil {
			b.Fatalf("bind error: %v", err)
		}
	}
}

func BenchmarkBind_File(b *testing.B) {
	// Build a multipart body *once* – the same body can be reused because
	// the binder only reads the headers, not the file contents.
	var body bytes.Buffer
	w := multipart.NewWriter(&body)

	fw, err := w.CreateFormFile("avatar", "avatar.jpg")
	if err != nil {
		b.Fatalf("multipart create error: %v", err)
	}
	// small dummy payload – the actual bytes are irrelevant for the binder.
	_, _ = io.Copy(fw, strings.NewReader("dummy-data"))
	w.Close()

	contentType := w.FormDataContentType()

	buf := bytes.NewReader(body.Bytes())

	req := httptest.NewRequest(http.MethodPost, "/", buf)
	req.Header.Set("Content-Type", contentType)

	var dst benchFile
	b.ReportAllocs()
	b.ResetTimer()

	req.Body = io.NopCloser(buf)

	for i := 0; i < b.N; i++ {
		buf.Seek(0, io.SeekStart)
		dst = benchFile{}
		// Reset Body for each iteration (the multipart parser consumes it).

		if err := Bind(req, &dst); err != nil {
			b.Fatalf("bind error: %v", err)
		}
	}
}

func BenchmarkBind_Form_URLEncoded(b *testing.B) {
	values := url.Values{}
	values.Set("title", "Bench Title")
	values.Set("count", "123")
	req := httptest.NewRequest(http.MethodPost,
		"/submit",
		strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var dst benchForm
	b.ReportAllocs()
	b.ResetTimer()

	buf := strings.NewReader(values.Encode())
	req.Body = io.NopCloser(buf)

	for i := 0; i < b.N; i++ {
		buf.Seek(0, io.SeekStart)
		dst = benchForm{}

		if err := Bind(req, &dst); err != nil {
			b.Fatalf("bind error: %v", err)
		}
	}
}

func BenchmarkBind_Form_Multipart(b *testing.B) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)

	_ = w.WriteField("title", "Multipart Bench")
	_ = w.WriteField("count", "777")
	w.Close()

	contentType := w.FormDataContentType()

	req := httptest.NewRequest(http.MethodPost, "/submit", bytes.NewReader(body.Bytes()))
	req.Header.Set("Content-Type", contentType)

	var dst benchForm
	b.ReportAllocs()
	b.ResetTimer()

	buf := bytes.NewReader(body.Bytes())
	req.Body = io.NopCloser(buf)

	for i := 0; i < b.N; i++ {
		buf.Seek(0, io.SeekStart)
		dst = benchForm{}

		if err := Bind(req, &dst); err != nil {
			b.Fatalf("bind error: %v", err)
		}
	}
}
