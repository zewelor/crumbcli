package main

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/browserutils/kooky"
)

func TestValidateFilters(t *testing.T) {
	for _, tt := range []struct {
		name    string
		filters Filters
		valid   bool
	}{
		{"domain", Filters{Domains: []string{"x.com"}}, true},
		{"multiple domains and names", Filters{Domains: []string{"x.com", "twitter.com"}, Names: []string{"auth_token", "ct0"}}, true},
		{"normalize domain", Filters{Domains: []string{".X.COM"}}, true},
		{"no domain", Filters{Names: []string{"sid"}}, false},
		{"empty domain", Filters{Domains: []string{""}}, false},
		{"url", Filters{Domains: []string{"https://x.com"}}, false},
		{"invalid label", Filters{Domains: []string{"x..com"}}, false},
		{"invalid name", Filters{Domains: []string{"x.com"}, Names: []string{"bad name"}}, false},
		{"empty name", Filters{Domains: []string{"x.com"}, Names: []string{""}}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateFilters(tt.filters)
			if (err == nil) != tt.valid {
				t.Fatal("unexpected validation result")
			}
			if tt.name == "normalize domain" && got.Domains[0] != "x.com" {
				t.Fatal("domain normalization failed")
			}
		})
	}
}

func TestDomainBoundary(t *testing.T) {
	for _, tt := range []struct {
		name, host string
		want       bool
	}{
		{"exact", "x.com", true}, {"leading dot", ".x.com", true}, {"subdomain", "api.x.com", true},
		{"case", "API.X.COM", true}, {"suffix confusion", "evilx.com", false}, {"superdomain", "com", false},
		{"embedded", "x.com.evil.test", false}, {"empty", "", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if domainMatches(tt.host, "x.com") != tt.want {
				t.Fatal("domain boundary mismatch")
			}
		})
	}
}

func testCookie(name, value, domain string) *kooky.Cookie {
	return &kooky.Cookie{Cookie: http.Cookie{Name: name, Value: value, Domain: domain, Path: "/", Secure: true, HttpOnly: true}}
}

func TestCookieSelection(t *testing.T) {
	now := time.Now()
	filters := Filters{Domains: []string{"x.com"}, Names: []string{"auth_token", "ct0"}}
	raw := []*kooky.Cookie{testCookie("auth_token", "synthetic-auth-secret", ".x.com"), testCookie("ct0", "synthetic-csrf-secret", "x.com"), testCookie("extra", "synthetic-extra-secret", "x.com"), testCookie("ct0", "unrelated-secret", "evilx.com")}
	got, err := selectCookies(raw, filters, now)
	if err != nil || len(got) != 2 {
		t.Fatal("complete selection failed")
	}
	for _, c := range got {
		if c.Domain == "evilx.com" || c.Name == "extra" {
			t.Fatal("unrelated cookie selected")
		}
	}
	raw[1].Value = ""
	got, err = selectCookies(raw, filters, now)
	if err != nil || len(got) != 2 || got[1].Value != "" {
		t.Fatal("empty cookie value was not preserved")
	}
	raw[1].Value = "synthetic-csrf-secret"
	raw[1].Expires = now.Add(-time.Hour)
	got, err = selectCookies(raw, filters, now)
	if err != nil || len(got) != 1 {
		t.Fatal("expired cookie was not omitted")
	}
}

func TestPartitionedCookieIsRejected(t *testing.T) {
	cookie := testCookie("sid", "synthetic-secret", "x.com")
	cookie.Partitioned = true
	if _, err := selectCookies([]*kooky.Cookie{cookie}, Filters{Domains: []string{"x.com"}}, time.Now()); err == nil {
		t.Fatal("partitioned cookie was silently flattened")
	}
}

func FuzzDomainBoundary(f *testing.F) {
	f.Add("api")
	f.Add("evil")
	f.Add("")
	f.Fuzz(func(t *testing.T, prefix string) {
		if prefix == "" || strings.ContainsAny(prefix, ".\x00\r\n") {
			t.Skip()
		}
		if domainMatches(prefix+"x.com", "x.com") {
			t.Fatal("unrelated suffix accepted")
		}
	})
}

func TestCookieValuesArePreserved(t *testing.T) {
	value := `{"synthetic":"private"}`
	raw := []*kooky.Cookie{testCookie("auth_token", "synthetic-auth-secret", "x.com"), testCookie("g_state", value, "x.com")}
	cookies, err := selectCookies(raw, Filters{Domains: []string{"x.com"}}, time.Now())
	if err != nil || len(cookies) != 2 {
		t.Fatal("JSON-valued cookie was omitted")
	}
	if cookies[1].Name != "g_state" || cookies[1].Value != value {
		t.Fatal("original value was not preserved")
	}
}
