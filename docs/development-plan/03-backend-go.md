# 03 — Backend (Go)

## 1. Dependensi

| Kebutuhan | Paket | Alasan |
|---|---|---|
| Router | `github.com/go-chi/chi/v5` | Kompatibel dengan `net/http`, middleware bersih, tanpa konsep asing |
| Driver PostgreSQL | `github.com/jackc/pgx/v5` | Pooling dan tipe bawaan yang matang |
| Pembangkit kueri | `sqlc` | SQL ditulis manual, kode Go dihasilkan bertipe |
| Migrasi | `github.com/pressly/goose/v3` | SQL polos, dapat dijalankan dari binary |
| Redis | `github.com/redis/go-redis/v9` | |
| Single-flight | `golang.org/x/sync/singleflight` | |
| Circuit breaker | `github.com/sony/gobreaker` | |
| Konfigurasi | `github.com/caarlos0/env/v11` | Konfigurasi lewat environment variable |
| Validasi | `github.com/go-playground/validator/v10` | |
| Uji integrasi | `github.com/testcontainers/testcontainers-go` | PostgreSQL sungguhan saat uji |
| Galat | `github.com/getsentry/sentry-go` | |

Logging memakai `log/slog` dari pustaka standar. Tidak perlu pustaka pihak ketiga.

## 2. Struktur proyek

```
.
├── cmd/
│   ├── api/main.go              # HTTP server
│   ├── ingestor/main.go         # scheduler ingestion
│   └── migrate/main.go          # runner goose
├── internal/
│   ├── config/                  # muat dan validasi env
│   ├── domain/                  # entity dan interface, tanpa dependensi luar
│   │   ├── match.go
│   │   ├── team.go
│   │   ├── player.go
│   │   ├── league.go
│   │   ├── article.go
│   │   └── errors.go
│   ├── service/                 # logika bisnis
│   │   ├── match.go
│   │   ├── standing.go
│   │   ├── article.go
│   │   └── search.go
│   ├── repository/
│   │   ├── gen/                 # dihasilkan sqlc, jangan disunting
│   │   └── postgres.go          # adapter ke interface domain
│   ├── provider/
│   │   ├── flashscore/          # anti-corruption layer
│   │   │   ├── scraper.go       # hasil/jadwal dari cjs.initialFeeds
│   │   │   └── details.go       # df_sui/df_st/df_li + parser ¬~ ¬ ÷
│   │   └── sofascore/           # penyedia cadangan
│   ├── cache/
│   │   ├── redis.go
│   │   └── swr.go               # stale-while-revalidate + single-flight
│   ├── stream/
│   │   ├── hub.go               # registry klien SSE
│   │   └── sse.go               # handler
│   ├── ingest/
│   │   ├── scheduler.go         # penjadwalan berjenjang
│   │   ├── differ.go            # bandingkan snapshot, hasilkan event
│   │   └── worker.go
│   └── transport/http/
│       ├── router.go
│       ├── handler/
│       ├── dto/                 # bentuk respons, terpisah dari domain
│       └── middleware/
├── db/
│   ├── migrations/              # goose
│   └── queries/                 # sumber sqlc
├── sqlc.yaml
├── docker-compose.yml
└── Makefile
```

Aturan ketergantungan, ditegakkan lewat `go vet` dan tinjauan kode:

```
transport → service → domain
repository → domain
provider   → domain
```

`domain` tidak mengimpor apa pun dari paket lain di proyek ini. Kalau suatu
saat `domain` perlu mengimpor `repository`, ada yang salah pada perancangan.

Bagi yang datang dari Spring: `domain` setara entity dan interface repository,
`service` setara `@Service`, `repository` setara implementasi JPA, dan
`transport/http/handler` setara `@RestController`. Perbedaan pentingnya, di Go
interface didefinisikan di sisi konsumen, bukan di sisi implementasi. Jadi
`service.MatchRepository` didefinisikan di dalam paket `service`, dan paket
`repository` memenuhinya tanpa perlu tahu.

## 3. Skema basis data

Migrasi pertama, disederhanakan agar terbaca:

```sql
-- kompetisi
CREATE TABLE leagues (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    flashscore_id TEXT UNIQUE,
    slug         TEXT NOT NULL UNIQUE,
    name         TEXT NOT NULL,
    country_code TEXT,
    tier         SMALLINT NOT NULL DEFAULT 2, -- 1 = prioritas lokal
    logo_url     TEXT,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE seasons (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    league_id  UUID NOT NULL REFERENCES leagues(id),
    name       TEXT NOT NULL,
    is_current BOOLEAN NOT NULL DEFAULT false
);

CREATE TABLE teams (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    flashscore_id TEXT UNIQUE,
    slug      TEXT NOT NULL UNIQUE,
    name      TEXT NOT NULL,
    short_name TEXT,
    logo_url  TEXT,
    venue     TEXT,
    founded   SMALLINT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE players (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    flashscore_id TEXT UNIQUE,
    slug        TEXT NOT NULL UNIQUE,
    name        TEXT NOT NULL,
    position    TEXT,
    nationality TEXT,
    birth_date  DATE,
    photo_url   TEXT,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- pertandingan
CREATE TYPE match_status AS ENUM (
    'scheduled','live','halftime','finished','postponed','cancelled'
);

CREATE TABLE matches (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    flashscore_id TEXT UNIQUE,
    season_id     UUID NOT NULL REFERENCES seasons(id),
    league_id     UUID NOT NULL REFERENCES leagues(id),
    home_team_id  UUID NOT NULL REFERENCES teams(id),
    away_team_id  UUID NOT NULL REFERENCES teams(id),
    kickoff_at    TIMESTAMPTZ NOT NULL,
    status        match_status NOT NULL DEFAULT 'scheduled',
    minute        SMALLINT,
    home_score    SMALLINT,
    away_score    SMALLINT,
    venue         TEXT,
    round         TEXT,
    slug          TEXT NOT NULL,               -- persib-vs-persija
    data_as_of    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_matches_kickoff  ON matches (kickoff_at DESC);
CREATE INDEX idx_matches_league   ON matches (league_id, kickoff_at DESC);
CREATE INDEX idx_matches_live     ON matches (status) WHERE status IN ('live','halftime');
CREATE INDEX idx_matches_team     ON matches (home_team_id, away_team_id);

CREATE TABLE match_events (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    match_id    UUID NOT NULL REFERENCES matches(id) ON DELETE CASCADE,
    flashscore_id TEXT,
    type        TEXT NOT NULL,        -- goal, own_goal, penalty, yellow, red, sub, var
    minute      SMALLINT NOT NULL,
    extra_minute SMALLINT,
    team_id     UUID REFERENCES teams(id),
    player_id   UUID REFERENCES players(id),
    related_player_id UUID REFERENCES players(id),
    detail      TEXT,
    UNIQUE (match_id, flashscore_id)
);

CREATE TABLE match_statistics (
    match_id UUID NOT NULL REFERENCES matches(id) ON DELETE CASCADE,
    team_id  UUID NOT NULL REFERENCES teams(id),
    stats    JSONB NOT NULL,          -- penguasaan bola, tembakan, dan sebagainya
    PRIMARY KEY (match_id, team_id)
);

CREATE TABLE lineups (
    match_id  UUID NOT NULL REFERENCES matches(id) ON DELETE CASCADE,
    team_id   UUID NOT NULL REFERENCES teams(id),
    player_id UUID NOT NULL REFERENCES players(id),
    is_starter BOOLEAN NOT NULL,
    shirt_number SMALLINT,
    position  TEXT,
    grid      TEXT,                   -- posisi pada formasi, contoh 4:2
    PRIMARY KEY (match_id, team_id, player_id)
);

CREATE TABLE standings (
    season_id  UUID NOT NULL REFERENCES seasons(id),
    team_id    UUID NOT NULL REFERENCES teams(id),
    position   SMALLINT NOT NULL,
    played     SMALLINT NOT NULL,
    won        SMALLINT NOT NULL,
    drawn      SMALLINT NOT NULL,
    lost       SMALLINT NOT NULL,
    goals_for  SMALLINT NOT NULL,
    goals_against SMALLINT NOT NULL,
    points     SMALLINT NOT NULL,
    form       TEXT,                  -- MMKSM
    zone       TEXT,                  -- champion, acl, relegation
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (season_id, team_id)
);

-- berita
CREATE TABLE articles (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug         TEXT NOT NULL UNIQUE,
    title        TEXT NOT NULL,
    excerpt      TEXT NOT NULL,
    body         TEXT NOT NULL,       -- markdown
    cover_url    TEXT,
    author_name  TEXT NOT NULL,
    status       TEXT NOT NULL DEFAULT 'draft',
    published_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_articles_published ON articles (published_at DESC)
    WHERE status = 'published';

-- inti pembeda produk: berita menempel pada entitas
CREATE TABLE article_entities (
    article_id  UUID NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    entity_type TEXT NOT NULL,        -- match, team, player, league
    entity_id   UUID NOT NULL,
    PRIMARY KEY (article_id, entity_type, entity_id)
);

CREATE INDEX idx_article_entities_lookup ON article_entities (entity_type, entity_id);

-- pencarian
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX idx_teams_name_trgm   ON teams   USING gin (name gin_trgm_ops);
CREATE INDEX idx_players_name_trgm ON players USING gin (name gin_trgm_ops);
```

Catatan perancangan:

- Seluruh PK memakai UUID (`gen_random_uuid()`), tidak bergantung penyedia.
  ID sumber disimpan di `flashscore_id TEXT UNIQUE`; ingestion melakukan upsert
  lewat `ON CONFLICT (flashscore_id)`. Penyedia baru cukup menambah kolom
  (mis. `sofascore_id`), PK dan FK tidak berubah.
- `match_statistics.stats` disimpan sebagai JSONB karena daftar statistik
  berbeda antarkompetisi dan berubah dari waktu ke waktu. Kolom tetap akan
  terus-menerus memaksa migrasi.
- `data_as_of` pada `matches` mengalir sampai ke antarmuka, mendukung
  penurunan bertingkat di [`02-architecture.md`](02-architecture.md).
- `article_entities` sengaja polimorfik tanpa foreign key, karena satu kolom
  tidak bisa mereferensikan empat tabel. Integritasnya dijaga di lapisan
  service.

## 4. Kontrak API

Awalan `/api/v1`. Rincian kontrak di [`api-contract-plan.md`](api-contract-plan.md);
bila bertentangan, dokumen itu yang berlaku. Waktu dalam RFC 3339 dengan zona
UTC; konversi ke zona pengguna dilakukan di klien.

### Format respons

```jsonc
// sukses, satu objek
{ "data": { ... }, "meta": { "data_as_of": "2026-09-14T10:32:00Z" } }

// sukses, koleksi
{
  "data": [ ... ],
  "meta": { "data_as_of": "...", "next_cursor": "eyJ..." }
}

// galat
{
  "error": {
    "code": "match_not_found",
    "message": "Pertandingan tidak ditemukan.",
    "request_id": "01J8X..."
  }
}
```

`code` bersifat stabil dan dapat diandalkan klien. `message` untuk manusia dan
boleh berubah. Setiap respons membawa header `X-Request-ID`.

### Endpoint

| Metode | Jalur | Keterangan |
|---|---|---|
| GET | `/api/v1/matches?date=2026-09-14&league=` | Dikelompokkan per kompetisi, kompetisi prioritas lebih dulu |
| GET | `/api/v1/matches/{id}` | Termasuk susunan pemain, statistik, kejadian, rekam pertemuan |
| GET | `/api/v1/matches/{id}/related-articles` | |
| GET | `/api/v1/leagues` | |
| GET | `/api/v1/leagues/{slug}/standings?season=` | |
| GET | `/api/v1/leagues/{slug}/matches?round=` | |
| GET | `/api/v1/teams/{slug}` | Profil, skuad, jadwal, hasil |
| GET | `/api/v1/teams/{slug}/matches?type=upcoming\|past` | |
| GET | `/api/v1/players/{slug}` | |
| GET | `/api/v1/articles?tag=&team=&cursor=` | |
| GET | `/api/v1/articles/{slug}` | |
| GET | `/api/v1/search?q=` | Klub, pemain, kompetisi |
| GET | `/api/v1/stream/matches?date=` | SSE |
| GET | `/api/v1/config` | Konfigurasi jarak jauh untuk klien |
| GET | `/healthz`, `/readyz` | |
| POST | `/api/v1/auth/login` | Set cookie sesi HttpOnly |
| GET/POST/PATCH | `/api/v1/admin/articles` | Cookie sesi dengan peran admin wajib |

### Format SSE

```
event: score
data: {"match_id":"0192f0c4-…","home_score":2,"away_score":1,"minute":67}

event: match_event
data: {"match_id":"0192f0c4-…","type":"goal","minute":67,"player":"Ciro Alves","team_id":"0192f0c5-…"}

event: status
data: {"match_id":"0192f0c4-…","status":"finished"}

: heartbeat
```

Komentar heartbeat dikirim tiap 20 detik agar proxy tidak memutus koneksi yang
dianggap menganggur.

## 5. Ingestion

### Penjadwalan berjenjang

| Jenis | Interval | Syarat |
|---|---|---|
| Pertandingan berlangsung | 20 detik | Hanya saat ada pertandingan berstatus live |
| Jadwal hari ini | 5 menit | |
| Klasemen | 30 menit | Hanya kompetisi yang ada pertandingan hari itu |
| Jadwal 7 hari ke depan | 6 jam | |
| Klub, pemain, skuad | 24 jam | |

Di luar jam pertandingan, ingestor nyaris menganggur. Ini penting agar volume
request ke Flashscore tetap wajar dan IP tidak diblokir. Detail pertandingan
(`df_sui`, `df_st`, `df_li`) hanya diambil saat status berubah, dengan jeda
minimal 1 detik antar-request.

### Deteksi perubahan

```go
// internal/ingest/differ.go

type Snapshot struct {
    Status     domain.MatchStatus
    Minute     int
    HomeScore  int
    AwayScore  int
    EventIDs   map[int64]struct{}
}

// Diff membandingkan kondisi tersimpan dengan yang baru diambil,
// lalu menghasilkan event yang layak disiarkan.
func Diff(old, new Snapshot) []domain.LiveEvent
```

Tiga hal yang harus ditangani, dan ketiganya adalah sumber bug tersering:

**Idempotensi.** Setiap event dikunci oleh `flashscore_id` dari penyedia (`III` pada feed
`df_sui`). Batasan
`UNIQUE (match_id, flashscore_id)` pada `match_events` membuat pengiriman ganda
gagal di tingkat basis data, bukan bergantung pada kedisiplinan kode.

**Revisi data.** Flashscore dapat menganulir gol lewat VAR, sehingga skor bisa
terlihat mundur. Penurunan skor tidak boleh diperlakukan sebagai gol baru.
Terbitkan event `score_correction`, jangan `goal`.

**State yang bertahan setelah restart.** Snapshot disimpan di PostgreSQL, bukan
hanya di memori. Ingestor yang restart tanpa state akan menyiarkan ulang seluruh
gol yang sudah terjadi.

### Ketahanan terhadap hulu

Setiap panggilan ke Flashscore wajib dibungkus:

```go
ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
defer cancel()

res, err := breaker.Execute(func() (any, error) {
    return scraper.Results(ctx)
})
```

Circuit breaker terbuka setelah lima kegagalan beruntun dan mencoba lagi setelah
30 detik. Respons `403` atau kosong dihitung gagal dan memicu peringatan
(kemungkinan `x-fsign` berubah). Tanpa batas waktu eksplisit, panggilan yang menggantung akan menumpuk
dan menjatuhkan seluruh proses.

## 6. Cache

```go
// internal/cache/swr.go

// Get mengembalikan nilai dari cache bila ada. Bila kedaluwarsa, nilai lama
// tetap dikembalikan dan penyegaran dijalankan di latar belakang. Bila kosong,
// hanya satu pemanggil yang menembus ke fn; sisanya menunggu hasil yang sama.
func (c *Cache) Get(
    ctx context.Context,
    key string,
    ttl, staleFor time.Duration,
    fn func(context.Context) ([]byte, error),
) ([]byte, error)
```

Konvensi kunci: `v1:matches:date:2026-09-14`, `v1:standings:liga-1:2026`.
Awalan versi memudahkan pembatalan massal saat bentuk data berubah.

Pembatalan berbasis event: ketika ingestor mendeteksi perubahan pada suatu
pertandingan, ia menghapus kunci terkait dan menerbitkan event. Cache tidak
perlu menunggu TTL habis.

## 7. Pengujian

| Lapisan | Pendekatan |
|---|---|
| `domain` | Uji unit murni, tanpa mock |
| `service` | Mock repository lewat interface yang didefinisikan di paket ini |
| `repository` | Testcontainers dengan PostgreSQL sungguhan, bukan sqlite |
| `provider` | Halaman dan feed Flashscore yang direkam sebagai berkas testdata; uji parser wajib |
| `transport` | `httptest` dengan service tiruan |
| `ingest/differ` | Uji tabel, dan di sinilah kepadatan kasus uji paling dibutuhkan |

Sasaran cakupan: 80 persen pada `service` dan `ingest`. Lapisan lain seadanya.
Angka cakupan bukan tujuan; `differ` adalah tempat bug paling mahal bersembunyi,
jadi ke sanalah usaha pengujian diarahkan.

## 8. Konfigurasi

Seluruhnya lewat environment variable, tanpa berkas konfigurasi di repositori.

```
APP_ENV, HTTP_PORT, DATABASE_URL, REDIS_URL,
FLASHSCORE_BASE_URL, FLASHSCORE_FSIGN, SCRAPE_MIN_DELAY,
SESSION_TTL, SENTRY_DSN, LOG_LEVEL,
INGEST_LIVE_INTERVAL, INGEST_ENABLED
```

`INGEST_ENABLED` berfungsi sebagai sakelar darurat. Kalau ingestion bermasalah
pada malam hari, ia dapat dimatikan tanpa deployment.
