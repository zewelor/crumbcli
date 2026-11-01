package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/browserutils/kooky"
)

func testApp(t *testing.T) (application, []string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Cookies"), []byte("synthetic profile"), 0600); err != nil {
		t.Fatal(err)
	}
	app := application{getenv: func(k string) string {
		if k == "HOME" {
			return dir
		}
		return ""
	}, read: func(context.Context, string, string, Filters) ([]*kooky.Cookie, error) {
		return []*kooky.Cookie{testCookie("auth_token", "synthetic-auth-secret", ".x.com"), testCookie("ct0", "synthetic-csrf-secret", "x.com")}, nil
	}}
	return app, []string{"--browser", "brave-origin", "--profile-dir", dir}
}

func TestExtractJSONToStdout(t *testing.T) {
	app, args := testApp(t)
	args = append(args, "--domain", "x.com")
	var out, stderr bytes.Buffer
	if app.run(args, &out, &stderr) != 0 {
		t.Fatalf("extraction failed: %s", stderr.String())
	}
	var got []Cookie
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal("stdout must be a cookie-record array")
	}
	want := []Cookie{
		{Name: "auth_token", Value: "synthetic-auth-secret", Domain: ".x.com", Path: "/", Secure: true, HTTPOnly: true},
		{Name: "ct0", Value: "synthetic-csrf-secret", Domain: "x.com", Path: "/", Secure: true, HTTPOnly: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("stdout did not preserve selected cookie fields and values")
	}
	if stderr.Len() != 0 {
		t.Fatal("successful extraction produced diagnostics")
	}
}

func TestNameFilter(t *testing.T) {
	app, args := testApp(t)
	args = append(args, "--domain", "x.com", "--name", "auth_token", "--name", "absent")
	var out, stderr bytes.Buffer
	if app.run(args, &out, &stderr) != 0 {
		t.Fatalf("name selection failed: %s", stderr.String())
	}
	var got []Cookie
	if json.Unmarshal(out.Bytes(), &got) != nil || len(got) != 1 || got[0].Name != "auth_token" || got[0].Value != "synthetic-auth-secret" {
		t.Fatal("name filter must select matching cookies without demanding all names")
	}
}

func TestEmptySelectionReturnsJSONArray(t *testing.T) {
	for _, mode := range []string{"empty", "expired"} {
		t.Run(mode, func(t *testing.T) {
			app, args := testApp(t)
			args = append(args, "--domain", "x.com")
			app.read = func(context.Context, string, string, Filters) ([]*kooky.Cookie, error) {
				if mode == "empty" {
					return nil, nil
				}
				cookie := testCookie("sid", "synthetic-secret", "x.com")
				cookie.Expires = time.Now().Add(-time.Hour)
				return []*kooky.Cookie{cookie}, nil
			}
			var out, stderr bytes.Buffer
			if app.run(args, &out, &stderr) != 0 || out.String() != "[]\n" || stderr.Len() != 0 {
				t.Fatal("empty selection must return an empty JSON array successfully")
			}
			if strings.Contains(stderr.String(), "synthetic-") {
				t.Fatal("diagnostics exposed a value")
			}
		})
	}
}

func TestInvalidUsageDoesNotReadBrowser(t *testing.T) {
	for _, flags := range [][]string{
		{"unexpected"}, {"--browser", "unknown", "--domain", "x.com"},
		{"--browser", "brave", "--unknown", "synthetic-secret"}, {"--domain", "x.com"},
		{"--browser", "brave", "--domain", "x.com", "--profile", "Default", "--profile-dir", "/tmp"},
		{"--browser", "brave", "--domain", "x.com", "--profile", "../other"},
	} {
		t.Run(strings.Join(flags, " "), func(t *testing.T) {
			app, _ := testApp(t)
			app.read = func(context.Context, string, string, Filters) ([]*kooky.Cookie, error) {
				t.Fatal("invalid invocation read browser")
				return nil, nil
			}
			var out, stderr bytes.Buffer
			if app.run(flags, &out, &stderr) != 2 || out.Len() != 0 {
				t.Fatal("invalid usage must exit 2 without cookie output")
			}
			if strings.Contains(stderr.String(), "synthetic-secret") {
				t.Fatal("invalid flag value leaked")
			}
		})
	}
}

func TestBrowserErrorsAreSanitized(t *testing.T) {
	app, args := testApp(t)
	args = append(args, "--domain", "x.com")
	app.read = func(context.Context, string, string, Filters) ([]*kooky.Cookie, error) {
		return nil, errors.New("synthetic-cookie-secret")
	}
	var out, stderr bytes.Buffer
	if app.run(args, &out, &stderr) != 1 || out.Len() != 0 || !strings.Contains(stderr.String(), "browser read failed") || strings.Contains(stderr.String(), "synthetic-cookie-secret") {
		t.Fatal("browser error was not safely reported")
	}
}

func TestCookieValuesSurviveJSONOutput(t *testing.T) {
	for _, tt := range []struct{ name, value string }{
		{"JSON", `{"synthetic":"private"}`},
		{"HTTP punctuation and controls", "synthetic;\"\\\r\n"},
		{"Unicode", "synthetic-zażółć-🍪"},
		{"literal replacement character", "synthetic-\ufffd"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app, args := testApp(t)
			args = append(args, "--domain", "x.com")
			app.read = func(context.Context, string, string, Filters) ([]*kooky.Cookie, error) {
				return []*kooky.Cookie{testCookie("g_state", tt.value, "x.com")}, nil
			}
			var out, stderr bytes.Buffer
			if app.run(args, &out, &stderr) != 0 {
				t.Fatal("cookie value blocked JSON extraction")
			}
			var got []Cookie
			if json.Unmarshal(out.Bytes(), &got) != nil || len(got) != 1 || got[0].Name != "g_state" || got[0].Value != tt.value {
				t.Fatal("cookie value changed during JSON export")
			}
			if stderr.Len() != 0 {
				t.Fatal("successful export emitted diagnostics")
			}
		})
	}
}

func TestInvalidUTF8FailsBeforeJSONOutput(t *testing.T) {
	for _, field := range []string{"name", "value", "domain", "path"} {
		t.Run(field, func(t *testing.T) {
			app, args := testApp(t)
			args = append(args, "--domain", "x.com")
			invalid := testCookie("sid", "synthetic-private-value", "x.com")
			switch field {
			case "name":
				invalid.Name = "synthetic-\xff"
			case "value":
				invalid.Value = "synthetic-\xff"
			case "domain":
				invalid.Domain = "synthetic-\xff.x.com"
			case "path":
				invalid.Path = "/synthetic-\xff"
			}
			app.read = func(context.Context, string, string, Filters) ([]*kooky.Cookie, error) {
				return []*kooky.Cookie{testCookie("valid", "synthetic-valid-value", "x.com"), invalid}, nil
			}
			var out, stderr bytes.Buffer
			if app.run(args, &out, &stderr) != 1 {
				t.Fatal("invalid UTF-8 must fail instead of changing cookie data")
			}
			if out.Len() != 0 {
				t.Fatal("invalid UTF-8 emitted partial cookie output")
			}
			if !strings.Contains(stderr.String(), "invalid UTF-8") || strings.Contains(stderr.String(), "synthetic-") {
				t.Fatal("invalid UTF-8 was not safely reported")
			}
		})
	}
}

func TestHelpAndVersion(t *testing.T) {
	for _, args := range [][]string{nil, {"--help"}, {"-h"}, {"version"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			app, _ := testApp(t)
			app.read = func(context.Context, string, string, Filters) ([]*kooky.Cookie, error) {
				t.Fatal("help read browser")
				return nil, nil
			}
			var out, stderr bytes.Buffer
			if app.run(args, &out, &stderr) != 0 || out.Len() == 0 || stderr.Len() != 0 {
				t.Fatal("help/version failed")
			}
		})
	}
}

func TestOutputFailure(t *testing.T) {
	app, args := testApp(t)
	args = append(args, "--domain", "x.com")
	var stderr bytes.Buffer
	if app.run(args, brokenOutput{}, &stderr) != 1 || !strings.Contains(stderr.String(), "cannot write result") {
		t.Fatal("output failure was ignored")
	}
}

type brokenOutput struct{}

func (brokenOutput) Write([]byte) (int, error) { return 0, errors.New("synthetic-output-secret") }

func TestExplicitProfileLocationDoesNotNeedHome(t *testing.T) {
	for _, mode := range []string{"directory", "XDG config"} {
		t.Run(mode, func(t *testing.T) {
			app, args := testApp(t)
			config := ""
			if mode == "XDG config" {
				config = t.TempDir()
				touch(t, filepath.Join(config, "BraveSoftware", "Brave-Origin", "Default", "Cookies"))
				args = []string{"--browser", "brave-origin"}
			}
			app.getenv = func(name string) string {
				if name == "XDG_CONFIG_HOME" {
					return config
				}
				return ""
			}
			args = append(args, "--domain", "x.com")
			var out, stderr bytes.Buffer
			if app.run(args, &out, &stderr) != 0 {
				t.Fatalf("explicit profile location failed without HOME: %s", stderr.String())
			}
			var got []Cookie
			if json.Unmarshal(out.Bytes(), &got) != nil || len(got) != 2 {
				t.Fatal("profile selection did not produce cookies")
			}
		})
	}
}
