package main

import (
	"errors"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/browserutils/kooky"
)

var domainLabel = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

type Filters struct {
	Domains []string
	Names   []string
}

type Cookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain"`
	Path     string `json:"path"`
	Secure   bool   `json:"secure"`
	HTTPOnly bool   `json:"httpOnly"`
}

func validateFilters(filters Filters) (Filters, error) {
	if len(filters.Domains) == 0 {
		return Filters{}, errors.New("--domain is required")
	}
	for i, domain := range filters.Domains {
		domain = strings.ToLower(strings.TrimPrefix(domain, "."))
		if !validDomain(domain) {
			return Filters{}, errors.New("invalid domain filter")
		}
		filters.Domains[i] = domain
	}
	for _, name := range filters.Names {
		if err := (&http.Cookie{Name: name, Value: "x"}).Valid(); err != nil {
			return Filters{}, errors.New("invalid cookie name filter")
		}
	}
	return filters, nil
}

func validDomain(domain string) bool {
	if len(domain) > 253 {
		return false
	}
	for _, label := range strings.Split(domain, ".") {
		if !domainLabel.MatchString(label) {
			return false
		}
	}
	return true
}

func domainMatches(host, domain string) bool {
	host = strings.ToLower(strings.TrimPrefix(host, "."))
	return host == domain || strings.HasSuffix(host, "."+domain)
}

func (r Filters) matches(c *kooky.Cookie) bool {
	if c == nil {
		return false
	}
	if len(r.Names) > 0 && !slices.Contains(r.Names, c.Name) {
		return false
	}
	for _, domain := range r.Domains {
		if domainMatches(c.Domain, domain) {
			return true
		}
	}
	return false
}

func selectCookies(raw []*kooky.Cookie, filters Filters, now time.Time) ([]Cookie, error) {
	result := []Cookie{}
	type key struct{ name, domain, path string }
	seen := map[key]Cookie{}
	for _, c := range raw {
		if !filters.matches(c) {
			continue
		}
		if c.Partitioned {
			return nil, errors.New("partitioned cookies cannot be represented")
		}
		if !c.Expires.IsZero() && !c.Expires.After(now) {
			continue
		}
		record := Cookie{Name: c.Name, Value: c.Value, Domain: c.Domain, Path: c.Path, Secure: c.Secure, HTTPOnly: c.HttpOnly}
		k := key{c.Name, c.Domain, c.Path}
		if prev, ok := seen[k]; ok {
			if prev != record {
				return nil, errors.New("conflicting cookies in selected profile")
			}
			continue
		}
		seen[k] = record
		result = append(result, record)
	}
	sort.Slice(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if a.Domain != b.Domain {
			return a.Domain < b.Domain
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.Name < b.Name
	})
	return result, nil
}
