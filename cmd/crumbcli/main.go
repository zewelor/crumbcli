package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/browserutils/kooky"
)

var version = "dev"

type browserReader func(context.Context, string, string, Filters) ([]*kooky.Cookie, error)
type application struct {
	read   browserReader
	getenv func(string) string
}

func main() {
	app := application{read: readBrowser, getenv: os.Getenv}
	os.Exit(app.run(os.Args[1:], os.Stdout, os.Stderr))
}

const usage = `CrumbCLI — read selected browser cookies as JSON on stdout.

Usage:
  crumbcli --browser BROWSER --domain DOMAIN [--name NAME]
  crumbcli version

--domain and --name can be repeated. A domain is required;
--name optionally restricts selection. Stdin is unused.
Profiles: --profile NAME or --profile-dir DIR.
Browsers: brave-origin, brave, chrome, chromium (Linux).
Output is a JSON array including cookie values. Diagnostics use stderr.
`

func (a application) run(args []string, out, errOut io.Writer) int {
	fail := func(code int, err error) int { fmt.Fprintln(errOut, "crumbcli:", err); return code }
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprint(out, usage)
		return 0
	}
	if args[0] == "version" && len(args) == 1 {
		fmt.Fprintln(out, version)
		return 0
	}
	fs := flag.NewFlagSet("crumbcli", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	opts := BrowserOptions{}
	fs.StringVar(&opts.Browser, "browser", "", "browser to read")
	fs.StringVar(&opts.Profile, "profile", "", "profile directory name")
	fs.StringVar(&opts.Directory, "profile-dir", "", "explicit profile directory")
	filters := Filters{}
	fs.Func("domain", "allowed domain (repeatable)", func(value string) error { filters.Domains = append(filters.Domains, value); return nil })
	fs.Func("name", "selected cookie name (repeatable)", func(value string) error { filters.Names = append(filters.Names, value); return nil })
	fs.Usage = func() {}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(out, usage)
			return 0
		}
		return fail(2, errors.New("invalid flags; use --help"))
	}
	if fs.NArg() != 0 {
		return fail(2, errors.New("positional arguments are not accepted; use --help"))
	}
	if err := opts.validate(); err != nil {
		return fail(2, err)
	}
	filters, err := validateFilters(filters)
	if err != nil {
		return fail(2, err)
	}
	file, err := profileFile(opts, a.getenv("HOME"), a.getenv("XDG_CONFIG_HOME"))
	if err != nil {
		return fail(1, err)
	}
	raw, err := a.read(context.Background(), file, opts.Browser, filters)
	if err != nil {
		return fail(1, errors.New("browser read failed; check selected profile and desktop keyring"))
	}
	cookies, err := selectCookies(raw, filters, time.Now())
	if err != nil {
		return fail(1, err)
	}
	if err := json.NewEncoder(out).Encode(cookies); err != nil {
		return fail(1, errors.New("cannot write result"))
	}
	return 0
}
