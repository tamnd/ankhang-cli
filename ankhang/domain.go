package ankhang

import (
	"context"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

func init() { kit.Register(Domain{}) }

// Domain is the An Khang Pharmacy kit driver.
type Domain struct{}

func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "ankhang",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "ankhang",
			Short:  "A command line for An Khang Pharmacy.",
			Long: `A command line for An Khang Pharmacy (ankhangpharma.com).

Fetches product details and category listings from one of Vietnam's
top pharmacy chains.
No API key required.`,
			Site: "https://" + Host,
			Repo: "https://github.com/tamnd/ankhang-cli",
		},
	}
}

func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	kit.Handle(app, kit.OpMeta{Name: "product", Group: "product", Single: true,
		URIType: "product", Resolver: true, Summary: "Fetch a product by slug or URL",
		Args: []kit.Arg{{Name: "ref", Help: "product slug (category/name) or URL"}}}, getProduct)

	kit.Handle(app, kit.OpMeta{Name: "products", Group: "product", List: true,
		URIType: "product", Summary: "List products from a category",
		Args: []kit.Arg{{Name: "category", Help: "category slug (e.g. thuoc-khong-ke-don)"}}}, listProducts)
}

func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := NewClientWithConfig(DefaultConfig())
	if cfg.UserAgent != "" {
		c.cfg.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.cfg.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.cfg.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.cfg.Timeout = cfg.Timeout
		c.http.Timeout = cfg.Timeout
	}
	return c, nil
}

// --- inputs ---

type productRef struct {
	Ref    string  `kit:"arg" help:"product slug or URL"`
	Client *Client `kit:"inject"`
}

type productsIn struct {
	Category string  `kit:"arg" help:"category slug (e.g. thuoc-khong-ke-don)"`
	Limit    int     `kit:"flag,inherit" help:"max results"`
	Client   *Client `kit:"inject"`
}

// --- handlers ---

func getProduct(ctx context.Context, in productRef, emit func(*Product) error) error {
	slug := productSlug(in.Ref)
	if slug == "" {
		return errs.Usage("unrecognized An Khang Pharmacy product reference: %q", in.Ref)
	}
	p, err := in.Client.GetProduct(ctx, slug)
	if err != nil {
		return err
	}
	return emit(p)
}

func listProducts(ctx context.Context, in productsIn, emit func(*Product) error) error {
	products, err := in.Client.ListProducts(ctx, in.Category, 1, in.Limit)
	if err != nil {
		return err
	}
	for _, p := range products {
		if err := emit(p); err != nil {
			return err
		}
	}
	return nil
}

// --- Resolver ---

func (Domain) Classify(input string) (uriType, id string, err error) {
	slug := productSlug(input)
	if slug != "" {
		return "product", slug, nil
	}
	return "", "", errs.Usage("unrecognized An Khang Pharmacy reference: %q", input)
}

func (Domain) Locate(uriType, id string) (string, error) {
	switch uriType {
	case "product":
		return baseURL + "/" + strings.Trim(id, "/") + ".html", nil
	default:
		return "", errs.Usage("ankhang has no resource type %q", uriType)
	}
}

// productSlug normalises any user input into a canonical slug (no .html).
func productSlug(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	if strings.Contains(input, "ankhangpharma.com") || strings.HasPrefix(input, "http") {
		return extractSlug(input)
	}
	slug := strings.Trim(input, "/")
	slug = strings.TrimSuffix(slug, ".html")
	if slug != "" && !strings.Contains(slug, " ") {
		return slug
	}
	return ""
}
