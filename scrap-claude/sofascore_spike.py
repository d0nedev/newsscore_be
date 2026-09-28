"""Spike SofaScore (Fase 0.3 + 0.4 di docs/development-plan/ingestion-multi-provider-plan.md).

Hasil (28 Sep 2026, IP Indonesia): /api/v1 selalu 403 ("Forbidden" untuk urllib, "challenge" untuk
curl_cffi dan Playwright). Yang berhasil: halaman HTML (__NEXT_DATA__) dan sitemap, lihat --ssr dan --sitemap.

Yang diuji:
  1. Akses API dari mesin ini (urllib biasa, lalu curl_cffi impersonate Chrome bila terpasang)
  2. Kategori Indonesia -> semua kompetisi -> musim berjalan
  3. (--deep) klasemen + pertandingan per kompetisi -> jumlah tim
  4. scheduled-events hari ini -> jumlah pertandingan dunia & Indonesia

Output: ringkasan di stdout, JSON mentah di sofascore_spike/ (bahan testdata).
Usage:
  python3 sofascore_spike.py            # katalog Indonesia + scheduled-events
  python3 sofascore_spike.py --deep     # + klasemen/pertandingan per kompetisi
  python3 sofascore_spike.py --browser  # lewat Chromium headless (Playwright), untuk lolos "challenge"
  python3 sofascore_spike.py --ssr      # tanpa API: baca __NEXT_DATA__ di HTML halaman (butuh curl_cffi)
  python3 sofascore_spike.py --sitemap  # daftar turnamen sepak bola suatu negara dari sitemap (butuh curl_cffi)
  pip install curl_cffi                 # opsional, bila urllib kena 403
  pip install playwright && playwright install chromium   # untuk --browser
"""
import argparse, json, os, time, urllib.error, urllib.request
from datetime import date

BASES = ["https://www.sofascore.com/api/v1", "https://api.sofascore.com/api/v1"]
HEADERS = {
    "User-Agent": "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0 Safari/537.36",
    "Accept": "application/json, text/plain, */*",
    "Accept-Language": "en-US,en;q=0.9",
    "Origin": "https://www.sofascore.com",
    "Referer": "https://www.sofascore.com/",
}
OUT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "sofascore_spike")
DELAY = 1.0  # detik antar request, jangan diturunkan


def get_urllib(url):
    req = urllib.request.Request(url, headers=HEADERS)
    try:
        with urllib.request.urlopen(req, timeout=30) as r:
            return r.status, r.read()
    except urllib.error.HTTPError as e:
        return e.code, e.read()


def get_cffi(url):
    from curl_cffi import requests  # impor di sini: opsional
    r = requests.get(url, headers=HEADERS, impersonate="chrome", timeout=30)
    return r.status_code, r.content


class BrowserFetch:
    """Panggil API dari dalam halaman sofascore.com supaya token challenge ikut terkirim."""

    def __init__(self, headless=True):
        from playwright.sync_api import sync_playwright  # impor di sini: opsional
        self._pw = sync_playwright().start()
        self._browser = self._pw.chromium.launch(headless=headless)
        ctx = self._browser.new_context(user_agent=HEADERS["User-Agent"], locale="en-US")
        self.page = ctx.new_page()
        self.page.goto("https://www.sofascore.com/football", wait_until="domcontentloaded", timeout=60000)
        self.page.wait_for_timeout(5000)  # beri waktu skrip challenge berjalan
        self.__name__ = "playwright"

    def __call__(self, url):
        r = self.page.evaluate(
            """async (url) => {
                const res = await fetch(url, {headers: {"Accept": "application/json"}});
                return [res.status, await res.text()];
            }""",
            url,
        )
        return r[0], r[1].encode()

    def close(self):
        self._browser.close()
        self._pw.stop()


class Client:
    def __init__(self, browser=None):
        self.base, self.fetch, self.requests = None, None, 0
        self.browser = browser

    def probe(self):
        """Cari kombinasi base URL + HTTP client yang tidak diblokir."""
        clients = [("urllib", get_urllib)]
        try:
            import curl_cffi  # noqa: F401
            clients.append(("curl_cffi", get_cffi))
        except ImportError:
            pass
        if self.browser:
            clients.append(("playwright", self.browser))
        print("== 1. Uji akses ==")
        for base in BASES:
            for name, fn in clients:
                try:
                    status, body = fn(base + "/sport/football/categories")
                except Exception as e:  # jaringan/TLS
                    status, body = f"error: {e}", b""
                self.requests += 1
                ok = status == 200 and body.lstrip().startswith(b"{")
                # reason "Forbidden" = ditolak di TLS/header; "challenge" = lolos TLS, butuh token challenge browser
                print(f"  {name:<9} {base:<36} -> {status}{'  OK' if ok else '  ' + body[:80].decode(errors='replace')}")
                if ok and not self.base:
                    self.base, self.fetch = base, fn
                    save("categories", json.loads(body))
                time.sleep(DELAY)
        if not self.base:
            hint = "" if len(clients) > 1 else " Coba: pip install curl_cffi"
            raise SystemExit(f"\nSemua kombinasi gagal/diblokir.{hint}")
        print(f"  dipakai: {getattr(self.fetch, '__name__', self.fetch)} @ {self.base}\n")

    def get(self, path, name=None):
        time.sleep(DELAY)
        status, body = self.fetch(self.base + path)
        self.requests += 1
        if status != 200:
            print(f"  ! {path} -> {status}")
            return None
        data = json.loads(body)
        if name:
            save(name, data)
        return data


def save(name, data):
    os.makedirs(OUT, exist_ok=True)
    with open(os.path.join(OUT, name + ".json"), "w") as f:
        json.dump(data, f, ensure_ascii=False, indent=2)


def ssr_page(path):
    """Ambil pageProps dari __NEXT_DATA__ (Next.js SSR). Halaman HTML tidak kena "challenge" seperti /api/v1."""
    import re
    from curl_cffi import requests
    time.sleep(DELAY)
    # Tanpa HEADERS: header Accept JSON membuat server tidak mengirim HTML lengkap
    r = requests.get("https://www.sofascore.com" + path, impersonate="chrome", timeout=30)
    status, body = r.status_code, r.content
    m = re.search(r'<script id="__NEXT_DATA__"[^>]*>(.*?)</script>', body.decode(errors="replace"), re.S)
    if status != 200 or not m:
        print(f"  ! {path} -> {status}, __NEXT_DATA__ {'ada' if m else 'tidak ada'}")
        return {}
    return json.loads(m.group(1))["props"]["pageProps"]


def run_ssr(start):
    """Mulai dari 1 halaman kompetisi -> tim dari klasemen -> halaman tim -> skuad + kompetisi lain yang diikuti."""
    print(f"== SSR: {start} ==")
    pp = ssr_page(start)
    ut = pp.get("uniqueTournament") or {}
    if not ut:
        raise SystemExit("  halaman kompetisi tidak berisi uniqueTournament")
    save(f"ssr_tournament_{ut['id']}", pp)
    seasons = pp.get("seasons") or []
    print(f"  {ut['name']} ut_id={ut['id']} category_id={ut['category']['id']} ({ut['category']['name']})")
    print(f"  musim: {len(seasons)}, terbaru {seasons[0]['year']} (season_id={seasons[0]['id']})" if seasons else "  musim: 0")
    teams = {}
    for group in pp.get("standings") or []:
        for r in group.get("rows", []):
            teams[r["team"]["id"]] = r["team"]
            print(f"   {r['position']:>2}. {r['team']['name']:<36} M={r.get('matches')} Pts={r.get('points')}")
    print(f"  tim dari klasemen: {len(teams)}\n")

    comps, n_players, with_dob = {}, 0, 0
    for t in teams.values():
        tp = ssr_page(f"/football/team/{t['slug']}/{t['id']}")
        if not tp:
            continue
        save(f"ssr_team_{t['id']}", tp)
        players = (tp.get("players") or {}).get("players", [])
        n_players += len(players)
        with_dob += sum(1 for p in players if p["player"].get("dateOfBirthTimestamp"))
        for c in (tp.get("teamUniqueTournaments") or {}).get("uniqueTournaments", []):
            comps.setdefault(c["id"], (c["name"], c["category"]["name"], set()))[2].add(t["name"])
        print(f"  {t['name']:<36} pemain={len(players)}")
    print(f"\n  total pemain: {n_players}, punya tanggal lahir: {with_dob}")
    print(f"\n  kompetisi yang diikuti tim-tim ini ({len(comps)}):")
    for cid, (name, cat, ts) in sorted(comps.items(), key=lambda x: -len(x[1][2])):
        print(f"   ut_id={cid:<6} {name:<40} [{cat}] {len(ts)} tim")
    save("ssr_competitions", {cid: {"name": n, "category": c, "teams": sorted(ts)} for cid, (n, c, ts) in comps.items()})


def run_sitemap(country_slug):
    """Daftar turnamen dari sitemap. Sitemap pertandingan (45 file, ±437 rb URL, ±63 MB) TIDAK lengkap."""
    import gzip, re
    from curl_cffi import requests
    url = "https://www.sofascore.com/sitemaps/en_sitemap_tournaments_football.xml.gz"
    body = requests.get(url, impersonate="chrome", timeout=60).content
    try:
        body = gzip.decompress(body)
    except OSError:
        pass
    locs = re.findall(r"<loc>(.*?)</loc>", body.decode(errors="replace"))
    mine = [u.replace("https://www.sofascore.com", "") for u in locs if f"/tournament/{country_slug}/" in u]
    print(f"== Sitemap: {len(locs)} turnamen sepak bola, {len(mine)} di '{country_slug}' ==")
    for u in mine:
        print(f"  {u}   (ut_id={u.rsplit('/', 1)[1]})")
    save(f"sitemap_tournaments_{country_slug}", mine)


def tournaments_of(data):
    # Bentuk respons bisa {"groups":[{"uniqueTournaments":[...]}]} atau {"uniqueTournaments":[...]}
    if "groups" in data:
        return [t for g in data["groups"] for t in g.get("uniqueTournaments", [])]
    return data.get("uniqueTournaments", [])


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--deep", action="store_true", help="ambil klasemen + pertandingan per kompetisi")
    ap.add_argument("--country", default="ID", help="kode alpha2 negara (default ID)")
    ap.add_argument("--browser", action="store_true", help="coba juga lewat Chromium headless (Playwright)")
    ap.add_argument("--headful", action="store_true", help="dengan --browser: tampilkan jendela browser")
    ap.add_argument("--ssr", nargs="?", const="/football/tournament/indonesia/super-league/1015",
                    help="mode tanpa API; argumen = path halaman kompetisi awal (default Super League)")
    ap.add_argument("--sitemap", nargs="?", const="indonesia", help="slug negara (default indonesia)")
    args = ap.parse_args()

    if args.sitemap:
        run_sitemap(args.sitemap)
        return
    if args.ssr:
        run_ssr(args.ssr)
        print(f"\nJSON mentah di {OUT}/")
        return

    browser = BrowserFetch(headless=not args.headful) if args.browser else None
    try:
        run(args, Client(browser))
    finally:
        if browser:
            browser.close()


def run(args, c):
    c.probe()

    print("== 2. Katalog kompetisi ==")
    cats = json.load(open(os.path.join(OUT, "categories.json")))["categories"]
    print(f"  total kategori/negara: {len(cats)}")
    cat = next((x for x in cats if x.get("alpha2") == args.country), None)
    if not cat:
        raise SystemExit(f"  kategori alpha2={args.country} tidak ditemukan")
    print(f"  {cat['name']}: category_id={cat['id']}")

    uts = tournaments_of(c.get(f"/category/{cat['id']}/unique-tournaments", "unique_tournaments") or {})
    print(f"  jumlah kompetisi: {len(uts)}\n")
    print(f"  {'ut_id':>7}  {'season_id':>9}  {'musim':<10} kompetisi")

    catalog = []
    for ut in uts:
        seasons = (c.get(f"/unique-tournament/{ut['id']}/seasons", f"seasons_{ut['id']}") or {}).get("seasons", [])
        cur = seasons[0] if seasons else {}
        row = {"ut_id": ut["id"], "name": ut["name"], "slug": ut.get("slug"),
               "season_id": cur.get("id"), "season": cur.get("year"), "seasons": len(seasons)}
        catalog.append(row)
        print(f"  {row['ut_id']:>7}  {str(row['season_id']):>9}  {str(row['season']):<10} {row['name']} ({row['seasons']} musim)")

    if args.deep:
        print("\n== 3. Tim per kompetisi (musim berjalan) ==")
        for row in catalog:
            if not row["season_id"]:
                continue
            ut, s = row["ut_id"], row["season_id"]
            teams = set()
            st = c.get(f"/unique-tournament/{ut}/season/{s}/standings/total", f"standings_{ut}_{s}")
            for group in (st or {}).get("standings", []):
                teams.update(r["team"]["id"] for r in group.get("rows", []))
            n_events = 0
            for kind in ("last", "next"):
                ev = c.get(f"/unique-tournament/{ut}/season/{s}/events/{kind}/0", f"events_{kind}_{ut}_{s}") or {}
                for e in ev.get("events", []):
                    n_events += 1
                    teams.update((e["homeTeam"]["id"], e["awayTeam"]["id"]))
            row.update(teams=len(teams), has_standings=bool(st), events_page0=n_events)
            print(f"  {row['name']:<40} tim={len(teams):>3}  klasemen={'ya' if st else 'tidak':<5}  pertandingan(hal.0)={n_events}")

    print("\n== 4. scheduled-events hari ini ==")
    today = date.today().isoformat()
    ev = (c.get(f"/sport/football/scheduled-events/{today}", f"scheduled_events_{today}") or {}).get("events", [])
    local = [e for e in ev if e.get("tournament", {}).get("category", {}).get("id") == cat["id"]]
    comps = {e.get("tournament", {}).get("uniqueTournament", {}).get("id") for e in ev}
    print(f"  {today}: {len(ev)} pertandingan dunia, {len(comps)} kompetisi, {len(local)} di {cat['name']}")
    for e in local:
        t = e["tournament"]
        print(f"    [{t.get('name')}] {e['homeTeam']['name']} vs {e['awayTeam']['name']} ({e['status']['type']})")

    save("catalog_summary", catalog)
    print(f"\nSelesai: {c.requests} request. JSON mentah di {OUT}/")


if __name__ == "__main__":
    main()
