package ankhang

import (
	"testing"

	"github.com/tamnd/any-cli/kit"
)

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "ankhang" {
		t.Errorf("Scheme = %q, want ankhang", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "ankhang" {
		t.Errorf("Identity.Binary = %q, want ankhang", info.Identity.Binary)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct{ in, typ, id string }{
		{"thuoc-khong-ke-don/paracetamol-500mg-stada", "product", "thuoc-khong-ke-don/paracetamol-500mg-stada"},
		{"thuoc-khong-ke-don/paracetamol-500mg-stada.html", "product", "thuoc-khong-ke-don/paracetamol-500mg-stada"},
		{"https://" + Host + "/my-pham/kem-chong-nang-anessa.html", "product", "my-pham/kem-chong-nang-anessa"},
	}
	for _, tc := range cases {
		typ, id, err := Domain{}.Classify(tc.in)
		if err != nil || typ != tc.typ || id != tc.id {
			t.Errorf("Classify(%q) = (%q, %q, %v), want (%q, %q, nil)",
				tc.in, typ, id, err, tc.typ, tc.id)
		}
	}
}

func TestLocate(t *testing.T) {
	got, err := Domain{}.Locate("product", "thuoc-khong-ke-don/paracetamol-500mg-stada")
	want := baseURL + "/thuoc-khong-ke-don/paracetamol-500mg-stada.html"
	if err != nil || got != want {
		t.Errorf("Locate = (%q, %v), want (%q, nil)", got, err, want)
	}
}

func TestLocateUnknownType(t *testing.T) {
	_, err := Domain{}.Locate("unknown", "x")
	if err == nil {
		t.Error("expected error for unknown type, got nil")
	}
}

func TestHostWiring(t *testing.T) {
	h, err := kit.Open()
	if err != nil {
		t.Fatal(err)
	}

	p := &Product{
		Slug:  "thuoc-khong-ke-don/paracetamol-500mg-stada",
		URL:   baseURL + "/thuoc-khong-ke-don/paracetamol-500mg-stada.html",
		Name:  "Paracetamol 500mg Stada",
		Price: 15000,
	}
	u, err := h.Mint(p)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	want := "ankhang://product/thuoc-khong-ke-don/paracetamol-500mg-stada"
	if u.String() != want {
		t.Errorf("Mint = %q, want %q", u.String(), want)
	}

	got, err := h.ResolveOn("ankhang", "my-pham/kem-chong-nang-anessa")
	if err != nil || got.String() != "ankhang://product/my-pham/kem-chong-nang-anessa" {
		t.Errorf("ResolveOn = (%q, %v), want ankhang://product/my-pham/kem-chong-nang-anessa", got.String(), err)
	}
}
