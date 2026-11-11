# Lineup Flashscore kosong sejak Oktober

Tanggal: 2026-11-11. Referensi: [`docs/ingestor.md`](../ingestor.md), [`p1-sofascore-sync-laptop.md`](p1-sofascore-sync-laptop.md).

## Item

| ID | Masalah | Perbaikan | File |
|---|---|---|---|
| L1 | Feed `df_li_1_<id>` menjawab `0` (kosong) untuk laga sejak awal Oktober 2026. `ScrapeMatchDetails` membuang error/hasil kosong lineup (`lineup, _ :=`), jadi statistik dan event tetap tersimpan dan job ditandai `done` tanpa lineup | `ScrapeLineup` membaca GraphQL `https://2.ds.lsapp.eu/pq_graphql?_hash=dlie2&eventId=<id>&projectId=2` (query yang dipakai tab Lineups di halaman laga). Parser `df_li` dihapus | `internal/provider/flashscore/details.go` |
| L2 | Laga yang sudah `done` tidak akan dibaca ulang | `ListMatchesToRereadLineups` juga memilih laga Flashscore selesai dengan statistik tapi tanpa lineup; `ingestor -backfill-lineups` membacanya | `db/queries/football_details.sql`, `internal/platform/database/sqlc/football_details.sql.go` |
| L3 | Backfill prod | Binary `ingestor -backfill-lineups` dijalankan sebagai container sekali-jalan `lineup-backfill` di network Coolify dengan env ingestor | server |

Pemetaan GraphQL ke `flashscore.Player`: `listName` jadi nama, `number` jadi nomor punggung, `teamName` jadi kebangsaan, `rating.value` jadi rating, `playerRoles[].title == "Goalkeeper"` jadi kiper, gambar `variantType` 83 (108x108, sama dengan `LPX` lama) jadi foto. Grup `STARTERS`/`SUBSTITUTES` (pelatih dilewati). Slot formasi dihitung dari `formation.lines[].rows[].playerIds` urut `sortKey`, kiper = 1. Nama formasi (`3-4-2-1`) langsung dari `formation.name`.

## Test

- `TestParseLineupGraphQL` dengan fixture dari respons asli (`testdata/lineup.json`, Crystal Palace v Liverpool, dipangkas): formasi, starter dengan slot dan rating, cadangan tanpa slot, kiper dari role, foto 108x108.
- `TestParseLineups` (format `df_li`) dihapus bersama parsernya.

## Verifikasi

- Sebelum: di prod, laga selesai minggu 12 Oktober sampai 2 November punya statistik 192 sampai 242 per minggu, lineup 0.
- `curl df_li_1_xEzJWyun`: 200, isi `0`. GraphQL `dlie2` untuk laga yang sama: 200, 58 KB, 40 pemain, tanpa header khusus.
- Live `flashscore.ScrapeLineup("xEzJWyun")`: formasi `3-4-2-1`/`4-2-3-1`, 40 pemain, slot 1 sampai 11, rating, foto.
- Laga `championship` (Indonesia): 23 dan 21 pemain, formasi null (sumber tidak memberi), tetap tersimpan.
- `go build ./...`, `go vet ./...`, `go test ./...`: ok. `sqlc diff`: bersih.
- Backfill di server: 2678 laga, sekitar 1 laga per detik. API `GET /api/v1/matches/1ff4d05a-...` mengembalikan `lineups.home` dengan `formationPlace` dan `stats.rating`.

## Temuan

- Backfill dari laptop lewat SSH tunnel sekitar 30 detik per laga (puluhan query per laga, latency tunnel). Dijalankan di server sebagai gantinya.
- Error lineup ditelan diam-diam di `ScrapeMatchDetails`. Kalau GraphQL berubah lagi, gejalanya sama (statistik ada, lineup tidak).

## Terbuka

- Ingestor prod masih versi lama (`df_li`): laga baru tetap tanpa lineup sampai branch di-merge ke `main` dan Coolify deploy. Sampai saat itu jalankan ulang container backfill.
- Peringatan saat lineup kosong untuk laga yang punya statistik belum ada.
- File sementara di server: `/root/ingestor-backfill`, `/root/backfill.env` (berisi kredensial DB, mode 600), container `lineup-backfill`.
