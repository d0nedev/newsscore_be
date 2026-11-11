"""SofaScore API through a real browser, for when the API answers 403 "challenge".

SofaScore asks flagged IPs (VPNs, heavy users) for a Cloudflare Turnstile
captcha: the site's JS solves it, trades it at /api/v1/token/captcha for a JWT
(60 minutes, bound to IP and browser fingerprint), and sends it as x-captcha
with every API call. A Go client cannot run that JS; Camoufox (Firefox) can.

This serves GET /api/v1/<path> on localhost by running fetch() inside a
sofascore.com page with the page's own token headers, and passes status and body
back. Point the ingestor or cmd/seed-sofascore at it: SOFASCORE_PROXY=http://localhost:8787

Turnstile rejects headless browsers: the window must be real, on a desktop or,
on a server, a virtual display (--virtual, needs Xvfb: apt install xvfb).

  pip install "camoufox[geoip]" && python -m camoufox fetch
  python3 scrap-claude/sofascore_proxy.py [--port 8787] [--host 127.0.0.1] [--virtual]
"""
import argparse, time
from http.server import BaseHTTPRequestHandler, HTTPServer

from camoufox.sync_api import Camoufox

HOME = "https://www.sofascore.com/"
TOKEN_TTL = 50 * 60  # the JWT lives 60 minutes; reload before it runs out

FETCH = """async ([path, headers]) => {
    const r = await fetch('/api/v1' + path, {headers});
    return [r.status, await r.text()];
}"""


class Browser:
    def __init__(self, virtual):
        self.virtual, self.cm = virtual, None
        self.launch()

    def launch(self):
        """A new browser, new fingerprint: Cloudflare stops handing tokens to one it has flagged."""
        if self.cm:
            self.cm.__exit__(None, None, None)
        self.cm = Camoufox(headless="virtual" if self.virtual else False, geoip=False)
        self.page = self.cm.__enter__().new_page()
        self.headers, self.at = {}, 0
        self.page.on("request", self.on_request)

    def on_request(self, r):
        # First call after refresh() only: our own fetches carry the token too and must not reset its age.
        if not self.headers and "/api/v1/" in r.url and "x-captcha" in r.headers:
            self.headers = {k: r.headers[k] for k in ("x-captcha", "x-requested-with")}
            self.at = time.time()

    def refresh(self):
        """Load the home page so the site's JS solves the captcha and calls the API with a token."""
        self.headers = {}
        self.page.goto(HOME, wait_until="domcontentloaded")
        for _ in range(60):
            if self.headers:
                print(f"token ok ({self.headers['x-captcha'][:24]}...)", flush=True)
                return
            self.page.wait_for_timeout(1000)
        print("no token after 60s: the page made no API call with x-captcha", flush=True)

    def retry(self, path):
        """Token revoked: reload for a new one; if this browser gets none, start a new browser."""
        self.refresh()
        if not self.headers:
            print("relaunching browser", flush=True)
            self.launch()
            self.refresh()
        return self.page.evaluate(FETCH, [path, self.headers])

    def get(self, path):
        if not self.headers or time.time() - self.at > TOKEN_TTL:
            self.refresh()
        status, body = self.page.evaluate(FETCH, [path, self.headers])
        if status == 403:  # token expired or revoked: one fresh try
            status, body = self.retry(path)
        return status, body


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--port", type=int, default=8787)
    ap.add_argument("--host", default="127.0.0.1", help="0.0.0.0 so Docker containers reach it; keep the port closed to the internet")
    ap.add_argument("--virtual", action="store_true", help="draw the browser on an Xvfb display (servers)")
    args = ap.parse_args()
    browser = Browser(args.virtual)

    class Handler(BaseHTTPRequestHandler):
        def do_GET(self):
            if not self.path.startswith("/api/v1/"):
                self.send_error(404)
                return
            try:
                status, body = browser.get(self.path[len("/api/v1"):])
            except Exception as e:  # browser crashed or page closed: let the caller retry later
                status, body = 502, str(e)
            data = body.encode()
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)

        def log_message(self, fmt, *a):
            print(f"{self.address_string()} {fmt % a}", flush=True)

    # Single-threaded on purpose: Playwright's sync page is not thread-safe, and
    # one request at a time is the pace SofaScore tolerates anyway.
    print(f"listening on http://{args.host}:{args.port}", flush=True)
    HTTPServer((args.host, args.port), Handler).serve_forever()


if __name__ == "__main__":
    main()
