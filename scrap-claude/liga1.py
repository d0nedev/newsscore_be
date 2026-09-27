"""Scrape Liga 1 Indonesia (Flashscore: "Super League") musim berjalan.

Output: liga1_results.json, liga1_fixtures.json, liga1_standings.json
Usage: python3 liga1.py
"""
import json, re, urllib.request
from datetime import datetime, timezone

BASE = "https://www.flashscore.com/football/indonesia/super-league/"
UA = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0 Safari/537.36"


def feed(page):
    req = urllib.request.Request(BASE + page + "/", headers={"User-Agent": UA})
    html = urllib.request.urlopen(req, timeout=30).read().decode()
    m = re.search(r"initialFeeds\['%s'\]\s*=\s*\{\s*data:\s*`(.*?)`" % page, html, re.S)
    if not m:
        raise RuntimeError(f"feed {page} tidak ditemukan, layout Flashscore berubah?")
    return parse(m.group(1))


def parse(data):
    # Format: record dipisah "¬~", field "KEY÷VALUE" dipisah "¬"
    out = []
    for rec in data.split("¬~"):
        f = dict(p.split("÷", 1) for p in rec.split("¬") if "÷" in p)
        if "AA" not in f:
            continue
        out.append({
            "id": f["AA"],
            "round": f.get("ER"),
            "kickoff": datetime.fromtimestamp(int(f["AD"]), timezone.utc).isoformat(),
            "home": f.get("AE"),
            "away": f.get("AF"),
            "home_score": int(f["AG"]) if f.get("AG", "").isdigit() else None,
            "away_score": int(f["AH"]) if f.get("AH", "").isdigit() else None,
        })
    return out


def standings(results):
    t = {}
    for m in results:
        if m["home_score"] is None:
            continue
        for team, gf, ga in ((m["home"], m["home_score"], m["away_score"]),
                             (m["away"], m["away_score"], m["home_score"])):
            r = t.setdefault(team, dict(team=team, p=0, w=0, d=0, l=0, gf=0, ga=0, pts=0))
            r["p"] += 1; r["gf"] += gf; r["ga"] += ga
            k = "w" if gf > ga else "d" if gf == ga else "l"
            r[k] += 1
            r["pts"] += {"w": 3, "d": 1, "l": 0}[k]
    # ponytail: tiebreak cuma pts/GD/GF; head-to-head resmi PSSI tidak dihitung
    rows = sorted(t.values(), key=lambda r: (-r["pts"], -(r["gf"] - r["ga"]), -r["gf"], r["team"]))
    for i, r in enumerate(rows, 1):
        r["pos"], r["gd"] = i, r["gf"] - r["ga"]
    return rows


def demo():
    rs = [dict(home="A", away="B", home_score=2, away_score=0),
          dict(home="B", away="C", home_score=1, away_score=1),
          dict(home="C", away="A", home_score=None, away_score=None)]
    s = standings(rs)
    assert [r["team"] for r in s] == ["A", "C", "B"] and s[0]["pts"] == 3 and s[2]["pts"] == 1


if __name__ == "__main__":
    demo()
    results, fixtures = feed("results"), feed("fixtures")
    table = standings(results)
    for name, obj in (("results", results), ("fixtures", fixtures), ("standings", table)):
        json.dump(obj, open(f"liga1_{name}.json", "w"), ensure_ascii=False, indent=2)
    print(f"{len(results)} hasil, {len(fixtures)} jadwal\n")
    print(f"{'#':>2} {'Tim':<22}{'M':>3}{'W':>3}{'D':>3}{'L':>3}{'GF':>4}{'GA':>4}{'GD':>4}{'Pts':>5}")
    for r in table:
        print(f"{r['pos']:>2} {r['team']:<22}{r['p']:>3}{r['w']:>3}{r['d']:>3}{r['l']:>3}{r['gf']:>4}{r['ga']:>4}{r['gd']:>4}{r['pts']:>5}")
