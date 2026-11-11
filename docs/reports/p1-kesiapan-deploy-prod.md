# Kesiapan deploy prod: branch feat/team-page-jobs

Tanggal: 2026-11-11. Referensi: [`docs/deployment.md`](../deployment.md), [`docs/runbook.md`](../runbook.md).

## Item

| ID | Masalah | Perbaikan | File |
|---|---|---|---|
| D1 | Docs deploy/ingestor belum di-commit | Commit `a56bf10` | `README.md`, `docs/deployment.md`, `docs/ingestor.md` |
| D2 | Hasil probe scraping ikut terlacak | Di-ignore | `.gitignore` (`/docs/scrap_result/`) |
| D3 | Branch belum ada di remote | Push `origin/feat/team-page-jobs`, fast-forward ke `main` | - |

## Test

Tidak ada test baru (hanya docs/config).

## Verifikasi

- `go build ./...`: OK
- `go vet ./...`: OK
- `go test ./...` dan `go test -race ./...`: semua `ok`
- `docker build .`: OK
- `docker compose --env-file .env.example config`: hanya menolak env wajib kosong (disengaja)
- Migrasi 0025 sampai 0030: hanya menambah kolom/tabel, `matches.flashscore_id` jadi nullable. Kompatibel dengan app lama.

## Temuan

- `0030_player_latest_club` adalah perbaikan data dengan down kosong. Backup DB wajib sebelum deploy.
- `deploy/docker-compose.yml` tidak meneruskan `SOFASCORE_PROXY`, jadi sync SofaScore dari server kena 403.

## Terbuka

- `govulncheck` belum dijalankan lokal (CI menjalankannya).
- Merge ke `main` dan smoke test `/ready` di server.
