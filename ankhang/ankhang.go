// Package ankhang is the library behind the ankhang command line:
// the HTTP client, HTML scraping, and typed data models for An Khang Pharmacy
// (www.ankhangpharma.com), one of Vietnam's top pharmacy chains.
//
// Product detail pages embed JSON-LD Product schema for structured data.
// Category listings are scraped from HTML link tags.
// Product URLs follow the pattern: https://www.ankhangpharma.com/{category}/{slug}.html.
package ankhang

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Host is the canonical site hostname.
const Host = "www.ankhangpharma.com"

// baseURL is the site root.
const baseURL = "https://www.ankhangpharma.com"

// DefaultUserAgent mimics a real browser.
const DefaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36"

// Config holds the tunable knobs for the HTTP client.
type Config struct {
	BaseURL   string
	Rate      time.Duration
	Retries   int
	Timeout   time.Duration
	UserAgent string
}

// DefaultConfig returns sensible production defaults.
func DefaultConfig() Config {
	return Config{
		BaseURL:   baseURL,
		Rate:      2 * time.Second,
		Retries:   3,
		Timeout:   30 * time.Second,
		UserAgent: DefaultUserAgent,
	}
}

// Client talks to the An Khang Pharmacy website over HTTP.
type Client struct {
	cfg  Config
	http *http.Client
	last time.Time
}

// NewClient returns a Client from DefaultConfig.
func NewClient() *Client { return NewClientWithConfig(DefaultConfig()) }

// NewClientWithConfig returns a Client built from cfg.
func NewClientWithConfig(cfg Config) *Client {
	return &Client{cfg: cfg, http: &http.Client{Timeout: cfg.Timeout}}
}

// Get fetches rawURL and returns the body bytes, pacing and retrying on transient errors.
func (c *Client) Get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.cfg.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, rawURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", rawURL, lastErr)
}

func (c *Client) do(ctx context.Context, rawURL string) ([]byte, bool, error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	req.Header.Set("Accept", "text/html,application/json,*/*")
	req.Header.Set("Referer", baseURL+"/")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	return b, err != nil, err
}

func (c *Client) pace() {
	if c.cfg.Rate <= 0 {
		return
	}
	if wait := c.cfg.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

// --- wire JSON-LD types ---

type wireJSONLD struct {
	Type    string           `json:"@type"`
	Name    string           `json:"name"`
	Desc    string           `json:"description"`
	Brand   wireJSONLDBrand  `json:"brand"`
	Offers  wireJSONLDOffer  `json:"offers"`
	Rating  wireJSONLDRating `json:"aggregateRating"`
}

type wireJSONLDBrand struct {
	Name string `json:"name"`
}

type wireJSONLDOffer struct {
	Price string `json:"price"`
}

type wireJSONLDRating struct {
	Value       string `json:"ratingValue"`
	ReviewCount string `json:"reviewCount"`
}

// --- public types ---

// Product is one An Khang Pharmacy product.
type Product struct {
	Slug                 string  `json:"slug"                       kit:"id" table:"slug"`
	Name                 string  `json:"name"                                table:"name"`
	URL                  string  `json:"url,omitempty"                       table:"url,url"`
	Price                float64 `json:"price"                               table:"price"`
	Category             string  `json:"category,omitempty"                  table:"category"`
	RegistrationNumber   string  `json:"registration_number,omitempty"       table:"reg_no"`
	Brand                string  `json:"brand,omitempty"                     table:"brand"`
	Manufacturer         string  `json:"manufacturer,omitempty"              table:"manufacturer"`
	OriginCountry        string  `json:"origin_country,omitempty"            table:"origin"`
	DosageForm           string  `json:"dosage_form,omitempty"               table:"dosage_form"`
	Strength             string  `json:"strength,omitempty"                  table:"strength"`
	PackSize             string  `json:"pack_size,omitempty"                 table:"pack_size"`
	PrescriptionRequired bool    `json:"prescription_required,omitempty"     table:"rx"`
	Rating               float64 `json:"rating,omitempty"                    table:"rating"`
	ReviewCount          int     `json:"review_count,omitempty"              table:"reviews"`
	FetchedAt            string  `json:"fetched_at,omitempty"                table:"fetched_at"`
}

// --- regexps ---

var jsonLdRE = regexp.MustCompile(`(?is)<script[^>]+type="application/ld\+json"[^>]*>([\s\S]*?)</script>`)
var productLinkRE = regexp.MustCompile(`href="(?:https://www\.ankhangpharma\.com)?/([a-z0-9-]+/[a-z0-9][a-z0-9-]+\.html)"`)
var regNoRE = regexp.MustCompile(`(?:VD|VN)-\d+-\d+`)

// --- client methods ---

// GetProduct fetches a product detail page by its slug (category/name-slug, no .html).
func (c *Client) GetProduct(ctx context.Context, slug string) (*Product, error) {
	base := c.cfg.BaseURL
	if base == "" {
		base = baseURL
	}
	slug = strings.Trim(slug, "/")
	slug = strings.TrimSuffix(slug, ".html")
	pageURL := base + "/" + slug + ".html"
	body, err := c.Get(ctx, pageURL)
	if err != nil {
		return nil, fmt.Errorf("product %s: %w", slug, err)
	}
	p := parseProductPage(body, slug, base)
	if p == nil {
		return &Product{Slug: slug, URL: pageURL, FetchedAt: time.Now().UTC().Format(time.RFC3339)}, nil
	}
	return p, nil
}

// ListProducts fetches products from a category listing page via HTML link scraping.
func (c *Client) ListProducts(ctx context.Context, categorySlug string, page, limit int) ([]*Product, error) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	}
	base := c.cfg.BaseURL
	if base == "" {
		base = baseURL
	}
	listURL := base + "/" + categorySlug + "?page=" + strconv.Itoa(page)
	body, err := c.Get(ctx, listURL)
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", categorySlug, err)
	}
	return parseListingPage(body, categorySlug, limit, base), nil
}

// --- parsers ---

func parseProductPage(body []byte, slug, base string) *Product {
	html := string(body)
	now := time.Now().UTC().Format(time.RFC3339)

	// Category is the first segment of the slug.
	category := ""
	if idx := strings.Index(slug, "/"); idx >= 0 {
		category = slug[:idx]
	}

	for _, m := range jsonLdRE.FindAllStringSubmatch(html, -1) {
		if len(m) < 2 {
			continue
		}
		var ld wireJSONLD
		if err := json.Unmarshal([]byte(m[1]), &ld); err != nil {
			continue
		}
		if ld.Type != "Product" {
			continue
		}
		price, _ := strconv.ParseFloat(strings.ReplaceAll(ld.Offers.Price, ",", ""), 64)
		rating, _ := strconv.ParseFloat(ld.Rating.Value, 64)
		reviewCount, _ := strconv.Atoi(ld.Rating.ReviewCount)

		// PrescriptionRequired: html contains "Thuốc kê đơn"
		rxRequired := strings.Contains(html, "Thuốc kê đơn")

		// RegistrationNumber: VD/VN number in html
		regNo := regNoRE.FindString(html)

		return &Product{
			Slug:                 slug,
			Name:                 ld.Name,
			URL:                  base + "/" + slug + ".html",
			Price:                price,
			Category:             category,
			Brand:                ld.Brand.Name,
			PrescriptionRequired: rxRequired,
			RegistrationNumber:   regNo,
			Rating:               rating,
			ReviewCount:          reviewCount,
			FetchedAt:            now,
		}
	}
	return nil
}

func parseListingPage(body []byte, categorySlug string, limit int, base string) []*Product {
	html := string(body)
	now := time.Now().UTC().Format(time.RFC3339)
	seen := map[string]bool{}
	var out []*Product

	for _, m := range productLinkRE.FindAllStringSubmatch(html, -1) {
		if len(m) < 2 {
			continue
		}
		rawPath := m[1] // e.g. "thuoc-khong-ke-don/paracetamol-500mg-stada.html"
		// Filter for categorySlug prefix.
		if !strings.HasPrefix(rawPath, categorySlug+"/") {
			continue
		}
		// Strip .html to get slug.
		slug := strings.TrimSuffix(rawPath, ".html")
		// Dedup.
		if seen[slug] {
			continue
		}
		seen[slug] = true

		out = append(out, &Product{
			Slug:      slug,
			URL:       base + "/" + rawPath,
			FetchedAt: now,
		})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

// extractSlug strips scheme+host, query string, .html, and leading/trailing slashes.
func extractSlug(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if idx := strings.Index(rawURL, "ankhangpharma.com/"); idx >= 0 {
		rawURL = rawURL[idx+len("ankhangpharma.com/"):]
	}
	if idx := strings.Index(rawURL, "?"); idx >= 0 {
		rawURL = rawURL[:idx]
	}
	rawURL = strings.TrimSuffix(rawURL, ".html")
	return strings.Trim(rawURL, "/")
}
