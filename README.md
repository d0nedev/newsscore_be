# newsscore

Backend NewsScore: skor langsung dan berita sepak bola Indonesia. Go ([chi](https://github.com/go-chi/chi)), PostgreSQL ([pgx](https://github.com/jackc/pgx) + [sqlc](https://sqlc.dev)), OpenTelemetry, deploy VPS. Data dari scraping Flashscore. Rencana: [`docs/development-plan/`](docs/development-plan/).

Kebutuhan: Go 1.27+ (memakai package `uuid` dari stdlib), Docker, [sqlc](https://sqlc.dev) untuk regenerate query.

## Arsitektur

```
cmd/server            entrypoint: HTTP server, graceful shutdown + readiness drain
internal/app          composition root: config, telemetry, DB pool, router (app.go); wiring domain (modules.go)
internal/ingest       ingestor: scrape Flashscore -> upsert PostgreSQL
internal/provider     anti-corruption layer penyedia data (flashscore)
internal/platform     utilitas sistem (bukan business logic):
  config              konfigurasi dari env / .env, validasi per environment
  database            pool Postgres + kode hasil generate sqlc
  middleware          request ID, access log, route tag (OTel), recovery, API key
  apperror, httpx     error terstruktur, JSON request/response
  health              /health (liveness) dan /ready (readiness + drain)
  logging, tracing, metrics, requestcontext  slog JSON, OTel traces & metrics via OTLP gRPC
db/migrations         migrasi golang-migrate (juga schema sumber sqlc)
db/queries            query SQL untuk sqlc
```

Urutan middleware: `ClientIP -> RequestID -> SecurityHeaders -> Logging -> RouteTag -> Recovery`, dengan rate limit (per IP klien) dan API key di `/api/v1`. Probe `/health` dan `/ready` tidak di-trace, dan hanya di-log saat gagal.

## API

Kontrak lengkap: [`openapi.yaml`](openapi.yaml). Base path: `/api/v1`. Semua error, termasuk 404/405, berformat `{"error":{"code":"...","message":"..."}}`.

| Method | Path | Auth | Keterangan |
|---|---|---|---|
| GET | `/health` | - | Liveness |
| GET | `/ready` | - | Readiness (ping DB; 503 saat drain) |
| GET | `/api/v1/matches?date=DD.MM.YYYY&leagueId=&teamId=&status=` | - | Pertandingan per tanggal (WIB) |
| GET | `/api/v1/matches/{id}` | - | Detail: kejadian, statistik, susunan pemain |
| GET | `/api/v1/matches/stream` | - | SSE: event `score` dan `match_event` |
| GET | `/api/v1/leagues` | - | Kompetisi (Super League, Championship, Piala Presiden) |
| GET | `/api/v1/leagues/{slug}?season=` | - | Kompetisi + klasemen, dihitung dari hasil |
| GET | `/api/v1/teams/{id}` | - | Profil tim, skuad, 5 laga terakhir/berikutnya |
| GET | `/api/v1/players/{id}` | - | Profil pemain, total musim, log pertandingan |
| GET | `/api/v1/search?q=` | - | Cari tim, pemain, berita (tahan salah ketik) |
| GET | `/api/v1/news?matchId=&teamId=&playerId=&cursor=&limit=` | - | Berita terbit, pagination cursor |
| GET | `/api/v1/news/{slug}` | - | Detail berita |
| POST | `/api/v1/auth/login` | - | Set cookie sesi; 10 percobaan/menit/IP |
| POST | `/api/v1/auth/logout` | - | Hapus sesi |
| GET | `/api/v1/me` | sesi | User yang login |
| GET, POST | `/api/v1/admin/news` | sesi admin | Daftar (termasuk draft), buat |
| GET, PUT, DELETE | `/api/v1/admin/news/{id}` | sesi admin | Baca, ganti, hapus |

- Rate limit per IP (`RATE_LIMIT_REQUESTS_PER_MINUTE`), melebihi batas mendapat `429 RATE_LIMITED`.
- Akun dibuat lewat CLI: `printf '%s\n' "$PASSWORD" | go run ./cmd/useradd -email you@example.com -name You [-admin]`.

Contoh request: folder [`http/`](http/).

## Menjalankan

### Semua via Docker Compose

```bash
make up          # postgres, migrate, app, otel-collector, jaeger, prometheus
```

- API: http://localhost:8080
- Jaeger: http://localhost:16686
- Prometheus: http://localhost:9090
- Grafana: http://localhost:3000 (dashboard `newsscore`, datasource Prometheus + Jaeger, tanpa login — dev only)
- Postgres: `localhost:55432` (postgres/postgres)

### App lokal

```bash
cp .env.example .env     # sesuaikan DB_*
make migrate-up DATABASE_URL='postgres://user:pass@localhost:5432/newsscore?sslmode=disable'
make run
go run ./cmd/ingestor   # scrape Flashscore tiap INGEST_INTERVAL; jalankan satu instans saja
```

## Konfigurasi

Lihat [`.env.example`](.env.example). Aturan validasi penting:

| Env | Aturan |
|---|---|
| `APP_ENV` | `development`, `staging`, atau `production` |
| `DB_SSL_MODE` | Default `require`; di `production` wajib `require`/`verify-ca`/`verify-full` |
| `OTEL_EXPORTER_OTLP_INSECURE` | Default `true` hanya di `development`, `false` di luar itu. Set `true` hanya untuk collector di network privat yang sama (lihat `deploy/`) |
| `OTEL_TRACE_SAMPLE_RATE` | 0–1, default `0.1` |
| `APP_*_TIMEOUT` | Default aman (5s/10s/10s/60s), harus > 0 |
| `APP_SHUTDOWN_DRAIN_DELAY` | Default `5s`; set lebih besar dari periode readiness probe |
| `APP_SHUTDOWN_TIMEOUT` | Default `10s`. `drain delay + timeout` harus < grace period orchestrator |
| `DB_CONNECT_TIMEOUT` | Default `30s`; lama retry koneksi DB pertama saat startup |
| `DB_STATEMENT_TIMEOUT` | Default `5s`; batas per query di sisi Postgres, harus < `APP_WRITE_TIMEOUT` |
| `.env` | Hanya dibaca jika `APP_ENV` kosong atau `development` |
| `TRUSTED_PROXIES` | CIDR proxy/LB di depan app, dipisah koma. Kosong = IP klien dari koneksi TCP. `X-Forwarded-For` hanya dipercaya jika koneksi datang dari CIDR ini. **Wajib diisi di belakang load balancer**, kalau tidak semua klien berbagi satu bucket rate limit. |

## Development

```bash
make test               # unit test (+ race detector)
make test-integration   # butuh Postgres; tiap test memakai schema terisolasi lalu dihapus
make lint               # gofmt + go vet
make vuln               # govulncheck
make sqlc               # regenerate setelah mengubah db/queries atau migrasi
make openapi-lint       # validasi openapi.yaml
make alerts-test        # promtool check + unit test alert rules
```

Domain baru: buat paket `internal/<domain>` (handler -> service -> sqlc) dan daftarkan route-nya di `internal/app/modules.go`. Error code umum ada di `internal/platform/apperror/codes.go`; code khusus domain ditaruh di `<domain>/errors.go`. Error query dipetakan dengan `database.IsNotFound` → `apperror.NotFound`, selain itu `tracing.Fail(span, apperror.Internal(...))`. Untuk beberapa query atomik pakai `pgx.BeginFunc` + `queries.WithTx`.

Migrasi baru: tambahkan pasangan `db/migrations/000N_nama.up.sql` dan `.down.sql`, lalu `make sqlc`. Jangan mengubah migrasi yang sudah pernah dijalankan.

CI (`.github/workflows/ci.yml`) menjalankan lint, govulncheck, `sqlc diff`, lint OpenAPI, test alert rules, migrasi up/down/up, test dengan Postgres, build image (dengan `VERSION`), dan scan Trivy.

## Operasional

Deploy production ke VPS (Caddy + Postgres + OTel Collector, observability ke Grafana Cloud): folder [`deploy/`](deploy/), langkah lengkap di [`docs/runbook.md`](docs/runbook.md#deploy-vps-deploy).

SLO, arti setiap alert, dan langkah penanganannya ada di [`docs/runbook.md`](docs/runbook.md). Alert rules: [`prometheus-alerts.yml`](prometheus-alerts.yml), dimuat oleh Prometheus di compose (http://localhost:9090/alerts).
