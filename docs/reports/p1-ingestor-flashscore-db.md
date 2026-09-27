# Laporan Pengerjaan: Ingestor Flashscore & Sinkronisasi DB
**Tanggal:** 27 September 2026
**Referensi:** Instruksi User, Arsitektur `03-backend-go.md`

## Item yang Dikerjakan

| ID | Masalah / Kebutuhan | Perbaikan / Implementasi | File |
|---|---|---|---|
| 1 | Skema DB Tim & Klasemen | Membuat migration SQLC untuk tabel `teams`, `matches`, `standings`. | `db/migrations/0003_football_tables.up.sql` |
| 2 | SQL Queries Upsert | Menulis query `UpsertTeam`, `UpsertMatch`, `UpsertStanding` & generate via `sqlc`. | `db/queries/football.sql`, `internal/platform/database/sqlc/*` |
| 3 | Domain Models | Membuat struct `Match`, `Team`, `Standing` sesuai skema. | `internal/domain/models.go` |
| 4 | Modul Scraper Permanen | Refaktor eksperimen *initialFeeds* Flashscore menjadi modul scraper bersih di backend. | `internal/provider/flashscore/scraper.go` |
| 5 | Scheduler Ingestor | Menulis fungsi worker sinkronisasi harian yang memetakan struct Flashscore ke parameter DB SQLC. | `internal/ingest/scheduler.go` |

## Test dan Bug yang Ditangkap
* **Bug pada ID Tim Flashscore:** ID bawaan Flashscore berupa string (cth: `AqJqcOYR`), sedangkan skema kita mewajibkan tipe `BIGINT`.
  * *Perbaikan:* Dibuatkan *Hash Function* `stringToID` yang mengonversi string konsisten menjadi `int64`.
* **Kompilasi Golang:** Impor `time` yang tidak terpakai secara eksplisit telah dihapus.
* **Uji Modul:** Eksekusi manual CLI berhasil mengekstrak 18 tim dan 28 pertandingan, dan klasemen dihitung sukses.

## Bukti Verifikasi
* **Kompilasi:** `go build ./internal/ingest/` (Sukses tanpa pesan error)
* **Sqlc Generate:** `sqlc generate` membuahkan tipe dan parameter (contoh: `UpsertMatchParams`) dengan presisi tinggi.

## Temuan Baru
Data statistik pemain (gol, *assist*, kartu merah) serta *match events* terbukti **tidak ditanam** dalam `initialFeeds['results']`. Data tersebut hanya bisa diambil via endpoint rahasia `x-fsign`. Oleh karena itu, scraper saat ini terfokus 100% pada pemetaan Entitas Utama (*Team, Match, Standing*).

## Item di Luar Cakupan (Masih Terbuka)
* Mengaitkan `ingest.Worker` ini ke dalam *Cron/Tick* di fungsi `main.go` agar berjalan otomatis setiap jam 00:00 (mungkin dengan pustaka `robfig/cron`).
* Mengambil data profil individu para *Player* (pemain).

