package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/browserutils/kooky"
	"github.com/browserutils/kooky/browser/brave"
	"github.com/browserutils/kooky/browser/chrome"
	"github.com/browserutils/kooky/browser/chromium"
)

var browsers = map[string]string{
	"brave-origin": "BraveSoftware/Brave-Origin",
	"brave":        "BraveSoftware/Brave-Browser",
	"chrome":       "google-chrome",
	"chromium":     "chromium",
}

type BrowserOptions struct{ Browser, Profile, Directory string }

func (opts BrowserOptions) validate() error {
	if opts.Browser == "" {
		return errors.New("--browser is required")
	}
	if _, ok := browsers[opts.Browser]; !ok {
		return errors.New("unsupported browser; use --help")
	}
	if opts.Profile != "" && opts.Directory != "" {
		return errors.New("use either --profile or --profile-dir")
	}
	if opts.Profile != "" && (opts.Profile == "." || opts.Profile == ".." || strings.ContainsAny(opts.Profile, "/\\")) {
		return errors.New("--profile must be a directory name; use --profile-dir for a path")
	}
	return nil
}

func profileFile(opts BrowserOptions, home, config string) (string, error) {
	root := browsers[opts.Browser]
	if opts.Directory == "" && home == "" && config == "" {
		return "", errors.New("HOME is required for browser profile discovery")
	}
	dir := opts.Directory
	if dir == "" {
		if config == "" {
			config = filepath.Join(home, ".config")
		}
		profile := opts.Profile
		if profile == "" {
			profile = "Default"
		}
		dir = filepath.Join(config, root, profile)
	}
	candidates := []string{filepath.Join(dir, "Cookies"), filepath.Join(dir, "Network", "Cookies")}
	files := []string{}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", errors.New("cannot inspect selected browser profile")
		}
		if !info.Mode().IsRegular() {
			return "", errors.New("cookie database is not a regular file")
		}
		files = append(files, candidate)
	}
	if len(files) != 1 {
		return "", errors.New("cookie database is missing or ambiguous in selected profile")
	}
	return files[0], nil
}

func readBrowser(ctx context.Context, file, browser string, filters Filters) ([]*kooky.Cookie, error) {
	if runtime.GOOS != "linux" {
		return nil, errors.New("this release supports Linux browser profiles")
	}
	filter := kooky.FilterFunc(filters.matches)
	switch browser {
	case "brave", "brave-origin":
		return brave.ReadCookies(ctx, file, filter)
	case "chrome":
		return chrome.ReadCookies(ctx, file, filter)
	case "chromium":
		return chromium.ReadCookies(ctx, file, filter)
	default:
		return nil, errors.New("unsupported browser")
	}
}
