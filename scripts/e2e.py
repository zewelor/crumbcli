#!/usr/bin/env python3
"""Hermetic binary → Kooky/SQLite → JSON stdout acceptance."""
import argparse
import json
import os
import selectors
import signal
from pathlib import Path
import sqlite3
import subprocess
import tempfile
import time


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    binary = args.binary.resolve()
    checks = []
    with tempfile.TemporaryDirectory(prefix="crumbcli-e2e-") as temporary:
        root = Path(temporary)
        profile = root / "profile"
        profile.mkdir()
        database = profile / "Cookies"
        connection = sqlite3.connect(database)
        connection.execute("CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT)")
        connection.execute("INSERT INTO meta VALUES ('version', '24')")
        connection.execute("CREATE TABLE cookies (creation_utc INTEGER NOT NULL, host_key TEXT NOT NULL, top_frame_site_key TEXT NOT NULL DEFAULT '', name TEXT NOT NULL, value TEXT NOT NULL, encrypted_value BLOB NOT NULL DEFAULT X'', path TEXT NOT NULL, expires_utc INTEGER NOT NULL, is_secure INTEGER NOT NULL, is_httponly INTEGER NOT NULL, UNIQUE(top_frame_site_key,host_key,name,path))")
        # Chromium stores microseconds since 1601-01-01 (Windows FILETIME / 10).
        epoch_offset = 11644473600
        future = (int(time.time() + 86400) + epoch_offset) * 1000000
        expired = (int(time.time() - 86400) + epoch_offset) * 1000000
        creation = (int(time.time()) + epoch_offset) * 1000000
        records = [("auth_token", "synthetic-auth-secret", ".x.com"), ("ct0", "synthetic-csrf-secret", "x.com"), ("sid", "synthetic-unrelated-secret", "evilx.com"), ("g_state", '{"synthetic":"private"}', "x.com"), ("empty", "", "x.com"), ("large", "synthetic-blocking-" + "x" * (256 << 10), "blocking.test"), ("expired", "synthetic-expired-secret", "expiry.test"), ("invalid", "synthetic-invalid-secret", "invalid.test")]
        for name, value, host in records:
            connection.execute("INSERT INTO cookies (creation_utc,host_key,name,value,encrypted_value,path,expires_utc,is_secure,is_httponly) VALUES (?,?,?,?,?,?,?,?,?)", (creation, host, name, value, sqlite3.Binary(b""), "/", expired if name == "expired" else future, 1, 1))
        connection.execute("UPDATE cookies SET value=CAST(? AS TEXT) WHERE host_key=?", (sqlite3.Binary(b"synthetic-invalid-secret-\xff"), "invalid.test"))
        connection.commit()
        connection.close()
        env = {**os.environ, "HOME": str(root)}
        canaries = [value for name, value, _ in records if value and name != "large"]
        base = [str(binary), "--profile-dir", str(profile)]
        argv = base + ["--browser", "brave-origin"]

        def run(flags, expected=0, browser="brave-origin"):
            result = subprocess.run(base + ["--browser", browser] + flags, stdin=subprocess.DEVNULL, text=True, capture_output=True, env=env, timeout=15)
            assert result.returncode == expected, "unexpected extraction exit status"
            assert all(secret not in result.stderr for secret in canaries), "cookie value appeared in diagnostics"
            if expected:
                assert not result.stdout, "failed selection emitted partial cookies"
                return None
            return json.loads(result.stdout), result.stderr

        for sig in (signal.SIGINT, signal.SIGTERM):
            process = subprocess.Popen(argv + ["--domain", "blocking.test"], stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=env)
            try:
                with selectors.DefaultSelector() as selector:
                    selector.register(process.stdout, selectors.EVENT_READ)
                    assert selector.select(timeout=5), "browser extraction produced no output before interruption"
                    assert os.read(process.stdout.fileno(), 1), "browser extraction ended before interruption"
                process.send_signal(sig)
                assert process.wait(timeout=5) == -sig, "signal did not terminate blocked output using OS defaults"
            finally:
                if process.poll() is None:
                    process.kill()
                process.communicate()
        checks.append("SIGINT and SIGTERM terminate blocked output using OS defaults")
        env.pop("HOME", None)
        cookies, diagnostic = run(["--domain", "x.com"])
        assert len(cookies) == 4 and not diagnostic
        assert {item["name"]: item["value"] for item in cookies} == {"auth_token": "synthetic-auth-secret", "ct0": "synthetic-csrf-secret", "g_state": '{"synthetic":"private"}', "empty": ""}, "cookie selection/value preservation failed"
        assert all(set(item) == {"name", "value", "domain", "path", "secure", "httpOnly"} for item in cookies)
        assert all(item["path"] == "/" and item["secure"] and item["httpOnly"] for item in cookies)
        checks.append("real Kooky Brave Origin SQLite extraction preserves JSON/empty values without HOME; unrelated domain excluded")
        for browser in ("brave", "chrome", "chromium"):
            selected, diagnostic = run(["--domain", "x.com"], browser=browser)
            assert selected == cookies and not diagnostic
        checks.append("Brave, Chrome and Chromium adapters read the same synthetic plaintext store")
        narrowed, diagnostic = run(["--domain", "x.com", "--name", "auth_token", "--name", "absent"])
        assert len(narrowed) == 1 and narrowed[0]["name"] == "auth_token" and not diagnostic
        checks.append("name filters select available matches without demanding every name")
        selected, _ = run(["--domain", "X.COM", "--domain", "twitter.com", "--name", "auth_token", "--name", "ct0"])
        assert selected == [cookie for cookie in cookies if cookie["name"] in {"auth_token", "ct0"}]
        checks.append("repeatable domain/name filters and domain normalization")
        process = subprocess.Popen(argv + ["--domain", "x.com"], stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=env)
        try:
            try:
                code = process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                raise AssertionError("extraction waited for unused stdin") from None
            stdout, stderr = process.communicate()
            assert code == 0 and json.loads(stdout) == cookies
            assert all(secret.encode() not in stderr for secret in canaries)
            checks.append("extraction finishes while stdin remains open and unwritten")
        finally:
            if process.poll() is None:
                process.kill()
            process.communicate()
        for flags in (["--domain", "unmatched.test"], ["--domain", "x.com", "--name", "absent"], ["--domain", "expiry.test"]):
            selected, diagnostic = run(flags)
            assert selected == [] and not diagnostic
        checks.append("empty domain/name selections and expired cookies return an empty JSON array successfully")
        run(["--domain", "https://x.com"], expected=2)
        run(["--domain", "x.com", "--name", "bad name"], expected=2)
        run([], expected=2)
        run(["--domain", "x.com", "--unknown"], expected=2)
        checks.append("invalid and missing filters and unknown flags rejected")
        run(["--domain", "x.com", "--domain", "invalid.test"], expected=1)
        checks.append("invalid UTF-8 from SQLite fails without emitting partial cookie output")
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps({"status": "passed", "checks": checks, "boundaries": "synthetic unpartitioned Chromium SQLite profile with plaintext values; no live keyring/decryption or upstream session validation"}, indent=2) + "\n")
    print(f"PASS: {len(checks)} hermetic binary scenarios; report: {args.output}")


if __name__ == "__main__":
    main()
