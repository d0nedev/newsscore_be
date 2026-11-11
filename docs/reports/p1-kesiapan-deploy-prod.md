# Kesiapan deploy prod: branch feat/team-page-jobs

Tanggal: 2026-11-11. Referensi: [`docs/deployment.md`](../deployment.md), [`docs/runbook.md`](../runbook.md).

## Item

| ID | Masalah | Perbaikan | File |
|---|---|---|---|
| D1 | Docs deploy/ingestor belum di-commit | Commit `a56bf10` | `README.md`, `docs/deployment.md`, `docs/ingestor.md` |
| D2 | Hasil probe scraping ikut terlacak | Di-ignore | `.gitignore` (`/docs/scrap_result/`) |
| D4 | `govulncheck` (CI) gagal: 17 vuln di stdlib go1.27.1 (net/http, http2, crypto/tls, crypto/x509, html/template, net/textproto) dan dep baru tls-client (circl 1.6.2, brotli 1.2.0, compress 1.18.2, x/net 0.58.0) | Go 1.27.4, circl 1.6.3, brotli 1.3.0, compress 1.20.4, x/net 0.62.0 | `go.mod`, `go.sum`, `Dockerfile`, `docs/deployment.md` |
| D3 | Branch belum ada di remote | Push `origin/feat/team-page-jobs`, fast-forward ke `main` | - |

## Test

Tidak ada test baru (hanya docs/config).

## Verifikasi

- `go build ./...`: OK
- `go vet ./...`: OK
- `go test ./...` dan `go test -race ./...`: semua `ok`
- `docker build .`: OK (ulang dengan `golang:1.27.4`)
- `govulncheck@v1.8.0 ./...`: 0 vuln terpanggil (sebelumnya 17, exit 3)
- `gofmt -l`, `sqlc diff`, `make openapi-lint` (3 warning unused component), `make alerts-test`: OK
- `docker compose --env-file .env.example config`: hanya menolak env wajib kosong (disengaja)
- Migrasi 0025 sampai 0030: hanya menambah kolom/tabel, `matches.flashscore_id` jadi nullable. Kompatibel dengan app lama.

## Temuan

- `0030_player_latest_club` adalah perbaikan data dengan down kosong. Backup DB wajib sebelum deploy.
- `deploy/docker-compose.yml` tidak meneruskan `SOFASCORE_PROXY`, jadi sync SofaScore dari server kena 403.

## Terbuka

- Job CI `test` (migrasi up/down/up di Postgres) dan scan Trivy image belum dijalankan lokal.
- Merge ke `main` dan smoke test `/ready` di server.
