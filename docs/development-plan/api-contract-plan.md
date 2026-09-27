# Kontrak API (Backend) - NewsScore

Sumber kebenaran: `app/types/api.ts`, `app/types/match.ts`, `app/services/api-client.ts`,
`app/utils/api-error.ts`, dan mock di `app/data/*` (setiap endpoint di bawah menggantikan satu helper mock).

Base URL: `NUXT_PUBLIC_API_BASE_URL` (mis. `http://localhost:8080`), prefix `/api/v1`.

---

## Konvensi Umum

- **Auth:** sesi di cookie **HttpOnly** yang di-set API (frontend kirim `credentials: "include"`, JS tidak pernah melihat token). Jadi **bukan** `Authorization: Bearer`.
- **CORS:** `Access-Control-Allow-Origin: <origin FE persis>` (bukan `*`) + `Access-Control-Allow-Credentials: true`. Expose header `X-Request-Id`.
- **ID:** string slug (`"liga-primer"`, `"arsenal"`), dipakai langsung di URL FE (`/sepak-bola/:league`, `/tim/:team`, `/pemain/:player`, `/pertandingan/:match`).
- **Tanggal:** FE saat ini pakai string `"DD.MM.YYYY"` (match log pemain `"DD.MM.YY"`, awal/akhir musim `"DD.MM"`). Backend boleh kirim format itu dulu; migrasi ke ISO 8601 butuh perubahan FE.
- **Field opsional** (`?` di tipe TS) boleh tidak dikirim / `null`.

### Sukses
```json
{ "data": { } }                                   // ApiResponse<T>
{ "data": [ ], "meta": { "nextCursor": "abc|null" } }  // CursorPage<T>, cursor opaque
```
Query pagination: `?cursor=<nextCursor>&limit=<n>`.

### Error (`ApiErrorBody`)
```json
{ "error": { "code": "NOT_FOUND", "message": "Match not found", "details": { "field": "pesan" } } }
```
Status yang dipetakan FE: `400/422` validation (`details` = map field -> string), `401`, `403`, `404`, `409`, `5xx`.
Selalu kirim header `X-Request-Id`.

---

## Model Data

```ts
type MatchStatus = "scheduled" | "live" | "finished";

Team        { id, name, badge }                       // badge = kode singkat, mis. "ARS"

Match {
  id, leagueId, status,
  time,            // jam kick-off (scheduled) atau menit berjalan (live), mis. "67'"
  date,            // "DD.MM.YYYY"
  round, venue,
  home: Team, away: Team,
  score: [number, number] | null,
  halfTime?: [number, number],
  events:  { minute, team: "home"|"away", type: "goal"|"yellow"|"red"|"sub", player, note?, assist? }[],
  stats:   { label, home, away, percent? }[],
  lineups: { home: LineupPlayer[], away: LineupPlayer[] },   // LineupPlayer { number, name, playerId? }
  headToHead: { date, label, score }[],
  odds?:        { bookmaker, home, draw, away }[],
  commentary?:  { minute: string, text, highlight? }[],
  playerStats?: { team, name, playerId?, rating, goals, assists, shots }[],
  referee?, attendance?: number, broadcasters?: string[]
}

League {
  id, country, name, season,
  type?: "league" | "cup" | "friendly",
  start?, end?,                       // "DD.MM"
  standings: StandingRow[],           // [] untuk piala/friendly
  archive: { season, winner, runnerUp }[]
}
StandingRow { position, teamId, team, played, won, drawn, lost, goalsFor, goalsAgainst, points, form: ("W"|"D"|"L")[] } // form terbaru dulu

TeamProfile extends Team {
  leagueId, venue, capacity, founded, city?,
  squad: { id, name, number, position: "GK"|"DF"|"MF"|"FW", age, matches, goals, assists }[],
  transfers?: { date, player, playerId?, direction: "in"|"out", club, fee }[]
}

PlayerProfile {
  id, name, teamId, position, number, age, country, height, foot: "Kiri"|"Kanan",
  birthDate?, marketValue?, contractUntil?, goalkeeper?,
  season: { matches, goals, assists, minutes },
  matchLog?: { date, competition, country, matchId?, home, away, score: [n,n],
               outcome: "W"|"D"|"L", rating?, minutes?, detail?, yellow?, red?, note? }[],
  career?: { group: "Liga"|"Piala Domestik"|"Piala Internasional"|"Tim Nasional",
             season, teamId?, team, competition, country, rating?, apps,
             savePct?, cleanSheets?, goals?, assists?, yellow, red }[],
  transfers: { season, date?, from, to, type?, fee }[],
  injuries:  { season, issue, from, to }[]
}

NewsItem { id, category, title, summary, published }  // published = label relatif ("2 jam lalu")
```

---

## Endpoint

### Navigasi / layout (`layouts/default.vue`, `DateNav.vue`)
| Method | Path | Response | Pengganti mock |
|---|---|---|---|
| GET | `/api/v1/meta` | `{ data: { today: "DD.MM.YYYY", matchDates: string[], liveCount: number, sports: string[] } }` | `today`, `matchDates`, `liveCount`, `sports` |
| GET | `/api/v1/countries` | `{ data: { name, slug }[] }` (hanya negara yang punya kompetisi) | `countries`, `findCountry` |

`liveCount` sebaiknya dipoll (atau pisah ke `/api/v1/matches/live/count`) karena berubah terus.

### Kompetisi
| Method | Path | Response |
|---|---|---|
| GET | `/api/v1/leagues?country=<slug>&type=league` | `{ data: League[] }` — ringkas boleh tanpa `archive`; `standings` dibutuhkan beranda (`StandingsOverview`) & sidebar |
| GET | `/api/v1/leagues/:id` | `{ data: League }` lengkap (standings + archive), 404 jika tidak ada |

Tanpa filter = semua kompetisi (`competitions`). `type=league` = hanya yang punya klasemen (`leagues`). `country` menggantikan `competitionsByCountry`.

### Pertandingan
| Method | Path | Response |
|---|---|---|
| GET | `/api/v1/matches?date=DD.MM.YYYY&leagueId=&teamId=&status=` | `CursorPage<Match>` — versi list boleh tanpa `events/stats/lineups/headToHead/odds/commentary/playerStats` |
| GET | `/api/v1/matches/:id` | `{ data: Match }` lengkap, 404 jika tidak ada |

Dipakai: beranda (`date`), halaman liga & negara (`leagueId`), halaman tim (`teamId`). Match status `live` perlu refresh berkala (polling / SSE nanti).

### Tim
| Method | Path | Response |
|---|---|---|
| GET | `/api/v1/teams/:id` | `{ data: TeamProfile }`, 404 jika tidak ada |

`MatchInfo` hanya butuh `capacity` tim tuan rumah — lebih murah kirim `capacity` langsung di `Match` detail.

### Pemain
| Method | Path | Response |
|---|---|---|
| GET | `/api/v1/players/:id` | `{ data: PlayerProfile & { team: Team } }`, 404 jika tidak ada |

Wajib ada untuk **setiap** pemain di `squad` tim: sekarang FE fallback ke entri squad bila profil tidak ada — backend sebaiknya selalu kembalikan profil (field opsional kosong). Sertakan `team` agar FE tidak perlu request kedua.

### Berita
| Method | Path | Response |
|---|---|---|
| GET | `/api/v1/news?matchId=&leagueId=&teamId=&cursor=&limit=` | `CursorPage<NewsItem>` |
| GET | `/api/v1/news/:id` | `{ data: NewsItem & { body: string } }` — belum ada halaman detail di FE |

Tab "Berita" di halaman pertandingan saat ini memakai list global; filter `matchId` untuk nanti.

### Pin (personalisasi)
FE saat ini hanya pin **liga** (`usePinnedLeagues`, state lokal, default liga pertama). Tim/negara belum ada di UI.

| Method | Path | Body | Response |
|---|---|---|---|
| GET | `/api/v1/me/pins` | – | `{ data: { leagues: string[] } }` |
| PUT | `/api/v1/me/pins/leagues/:id` | – | `204` (idempotent) |
| DELETE | `/api/v1/me/pins/leagues/:id` | – | `204` |

Tanpa login: `401`, FE tetap pakai state lokal.

### Auth (belum ada UI, dibutuhkan untuk pin di server)
| Method | Path | Body | Response |
|---|---|---|---|
| POST | `/api/v1/auth/login` | `{ email, password }` | `{ data: { user: { id, email, name } } }` + `Set-Cookie` HttpOnly, Secure, SameSite |
| POST | `/api/v1/auth/logout` | – | `204`, hapus cookie |
| GET | `/api/v1/me` | – | `{ data: { id, email, name } }` atau `401` |

Validasi gagal: `422` dengan `details: { email: "...", password: "..." }`.
