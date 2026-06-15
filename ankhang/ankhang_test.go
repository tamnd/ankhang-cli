package ankhang

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestClient(srv *httptest.Server) *Client {
	cfg := DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0
	cfg.Retries = 0
	return NewClientWithConfig(cfg)
}

func sampleProductPageHTML(name, brand, regNo string, price float64, rxRequired bool) string {
	ld, _ := json.Marshal(map[string]any{
		"@type": "Product",
		"name":  name,
		"brand": map[string]string{"name": brand},
		"offers": map[string]any{
			"price": fmt.Sprintf("%.0f", price),
		},
		"aggregateRating": map[string]string{
			"ratingValue": "4.4",
			"reviewCount": "156",
		},
	})
	body := `<!DOCTYPE html><html><head>
<script type="application/ld+json">` + string(ld) + `</script>
</head><body><h1>` + name + `</h1>`
	if rxRequired {
		body += `<span>Thuốc kê đơn</span>`
	}
	if regNo != "" {
		body += `<p>Số đăng ký: ` + regNo + `</p>`
	}
	body += `</body></html>`
	return body
}

func sampleListingPageHTML(categorySlug string, slugs []string) string {
	body := `<!DOCTYPE html><html><body>`
	for _, slug := range slugs {
		body += fmt.Sprintf(`<a href="/%s/%s.html">%s</a>`, categorySlug, slug, slug)
	}
	body += `</body></html>`
	return body
}

func TestGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request carried no User-Agent")
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Errorf("body = %q, want ok", body)
	}
}

func TestGetRetriesOn503(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("recovered"))
	}))
	defer srv.Close()

	cfg := DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0
	cfg.Retries = 5
	c := NewClientWithConfig(cfg)

	start := time.Now()
	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "recovered" {
		t.Errorf("body = %q after retries", body)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}

func TestGetProduct(t *testing.T) {
	html := sampleProductPageHTML("Paracetamol Stada 500mg", "Stada", "VD-12345-20", 15000, true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(html))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	p, err := c.GetProduct(context.Background(), "thuoc-khong-ke-don/paracetamol-500mg-stada")
	if err != nil {
		t.Fatal(err)
	}
	if p.Slug != "thuoc-khong-ke-don/paracetamol-500mg-stada" {
		t.Errorf("Slug = %q", p.Slug)
	}
	if p.Name != "Paracetamol Stada 500mg" {
		t.Errorf("Name = %q", p.Name)
	}
	if p.Brand != "Stada" {
		t.Errorf("Brand = %q", p.Brand)
	}
	if p.Rating != 4.4 {
		t.Errorf("Rating = %v, want 4.4", p.Rating)
	}
	if p.ReviewCount != 156 {
		t.Errorf("ReviewCount = %d, want 156", p.ReviewCount)
	}
	if !p.PrescriptionRequired {
		t.Error("PrescriptionRequired should be true")
	}
	if p.RegistrationNumber != "VD-12345-20" {
		t.Errorf("RegistrationNumber = %q, want VD-12345-20", p.RegistrationNumber)
	}
}

func TestListProducts(t *testing.T) {
	html := sampleListingPageHTML("thuoc-khong-ke-don", []string{
		"paracetamol-500mg-stada", "vitamin-c-1000mg", "ibuprofen-400mg",
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(html))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	products, err := c.ListProducts(context.Background(), "thuoc-khong-ke-don", 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(products) != 3 {
		t.Fatalf("got %d products, want 3", len(products))
	}
	if products[0].Slug != "thuoc-khong-ke-don/paracetamol-500mg-stada" {
		t.Errorf("products[0].Slug = %q", products[0].Slug)
	}
}

func TestListProductsLimit(t *testing.T) {
	html := sampleListingPageHTML("thuoc-khong-ke-don", []string{
		"paracetamol-500mg-stada", "vitamin-c-1000mg", "ibuprofen-400mg",
		"omeprazole-20mg", "amoxicillin-500mg",
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(html))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	products, err := c.ListProducts(context.Background(), "thuoc-khong-ke-don", 1, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(products) != 3 {
		t.Errorf("got %d products, want 3 (limit respected)", len(products))
	}
}

func TestExtractSlug(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://www.ankhangpharma.com/thuoc-khong-ke-don/paracetamol-500mg-stada.html", "thuoc-khong-ke-don/paracetamol-500mg-stada"},
		{"https://www.ankhangpharma.com/my-pham/kem-chong-nang-anessa.html", "my-pham/kem-chong-nang-anessa"},
		{"thuoc-khong-ke-don/paracetamol-500mg-stada", "thuoc-khong-ke-don/paracetamol-500mg-stada"},
		{"thuoc-khong-ke-don/paracetamol-500mg-stada.html", "thuoc-khong-ke-don/paracetamol-500mg-stada"},
		{"", ""},
	}
	for _, tc := range cases {
		got := extractSlug(tc.in)
		if got != tc.want {
			t.Errorf("extractSlug(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseProductPageNoJSONLD(t *testing.T) {
	html := `<html><body><h1>No JSON-LD here</h1></body></html>`
	p := parseProductPage([]byte(html), "thuoc-khong-ke-don/some-product", "https://www.ankhangpharma.com")
	if p != nil {
		t.Errorf("expected nil for page without JSON-LD, got %+v", p)
	}
}
