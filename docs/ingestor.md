# Ingestor

`cmd/ingestor` mengisi PostgreSQL dari Flashscore dan SofaScore. API (`cmd/server`) hanya membaca hasilnya. Kode: `internal/ingest` (alur dan penyimpanan), `internal/provider/flashscore` dan `internal/provider/sofascore` (request dan parsing).

Jalankan **satu instans saja**: dua instans men-scrape dua kali dan menggandakan risiko diblokir. Tugas housekeeping (hapus sesi login kedaluwarsa) juga menumpang di sini karena alasan yang sama.

```bash
go run ./cmd/ingestor                      # loop terus sampai SIGINT/SIGTERM
go run ./cmd/ingestor -backfill-lineups    # sekali jalan, lalu keluar
go run ./cmd/seed-sofascore                # isi awal kompetisi SofaScore
go run ./cmd/seed-sofascore -images        # hanya unduh logo/foto yang belum ada
```

## Dua loop

Keduanya jalan langsung saat start, lalu tiap interval. Dipisah supaya sync liga yang lambat tidak menunda skor live.

| Loop | Interval | Isi |
|---|---|---|
| `league` (`Worker.Sync`) | `INGEST_INTERVAL` (default 5m, min 1m) | Semua langkah di bagian berikut |
| `live` (`Worker.SyncLive`) | `INGEST_LIVE_INTERVAL` (default 20s, min 10s) | Skor, babak, dan kejadian pertandingan yang sedang main |

Satu langkah yang gagal hanya di-log; langkah berikutnya tetap jalan.

## Loop league, urut per langkah

1. **Halaman liga Flashscore**: tiap kompetisi `active AND scraped` dengan `flashscore_path`. Simpan tim, jadwal, dan hasil, plus logo liga dan apakah musimnya lintas tahun (`split_season`). Halaman hanya memuat 100 laga; sisa musim (halaman "show more", 2-6 request tambahan) dibaca sekali per 24 jam per liga.
2. **Timnas**: tim dari negara dengan `scrape_national_team` (Indonesia). Semua laganya di semua kompetisi, termasuk uji coba, disimpan; kompetisinya dibuat otomatis, tapi laga lain di kompetisi itu tidak diambil.
3. **Skuad Flashscore**: maks 10 tim per run, tiap tim sekali sehari. Sumber posisi pemain (lineup hanya menandai kiper) dan logo HD tim. Skuad timnas hanya mengisi posisi; klub dan nomor punggung pemain tidak berubah.
4. **Gambar**: logo liga, logo tim, dan maks 100 foto pemain per run diunduh ke `ASSETS_DIR` (`leagues/`, `teams/`, `players/`), ditulis atomik, dan disajikan API di `/assets/...`. Sumber yang tidak punya gambar dicatat `""` agar tidak ditanya lagi; URL sumber baru mengosongkannya sehingga diunduh ulang.
5. **SofaScore** (maks sekali per jam): kompetisi `active` dengan `sofascore_id` dan tanpa `flashscore_path`, yaitu liga Indonesia yang tidak ada di Flashscore (Liga Nusantara, EPA, Liga Topskor). Per kompetisi: laga musim terbaru, klasemen (snapshot diganti utuh), dan top players per kategori. Lalu maks 20 skuad tim SofaScore, tiap tim sekali seminggu. Begitu SofaScore menjawab 403, sisa run SofaScore dihentikan sampai jam berikutnya.
6. **Job queue**: lihat bagian berikut.

Antar request ke sumber ada jeda 1 detik.

## Job queue (`ingest_jobs`)

Pekerjaan per target yang mahal lewat antrean di tabel `ingest_jobs` (unik per `kind, target`). Tiap run, ingestor mengantrekan job baru lalu mengerjakan maks `INGEST_DETAILS_PER_RUN` job yang sudah jatuh tempo, yang paling lama dulu. `INGEST_DETAILS_PER_RUN=0` mematikan seluruh antrean.

| `kind` | `target` | Diantrekan saat | Isi |
|---|---|---|---|
| `match_details` | Flashscore match id | Laga selesai tanpa statistik, musim berjalan kompetisinya | Kejadian, statistik, lineup (slot formasi, rating), formasi |
| `sofascore_match` | SofaScore event id | Laga SofaScore selesai tanpa statistik | Kejadian, statistik, lineup dengan statistik per pemain, formasi |
| `team_page` | Flashscore team id | Tiap klub di kompetisi yang di-scrape, sekali seminggu | Hasil dan jadwal klub di kompetisi yang di-scrape (melengkapi batas 100 laga halaman liga). Laga uji coba dan kompetisi lain dilewati |

Statistik disimpan paling akhir: barisnya menandai laga sudah lengkap, jadi kegagalan di tengah membuat laga diantrekan ulang. Job `done` yang statistiknya hilang lagi (mis. dihapus migrasi) otomatis diantrekan ulang.

Kegagalan:

- Error biasa: dicoba lagi setelah 5, 10, 20, lalu 40 menit. Setelah percobaan ke-5 status jadi `failed` dan tidak diantrekan lagi; pesan terakhir ada di `last_error`.
- 403/429 dari sumber (`ErrBlocked`): job ini dan semua sisa job yang sudah diambil ditunda 15 menit, run berhenti.
- Ingestor mati di tengah job: job `running` yang tidak bergerak 15 menit diambil lagi.

Progres antrean juga di-log tiap run (`job progress`):

```sql
SELECT kind, status, count(*) FROM ingest_jobs GROUP BY 1, 2;
SELECT kind, target, attempts, last_error FROM ingest_jobs WHERE status = 'failed';
-- Coba lagi semua yang failed:
UPDATE ingest_jobs SET status = 'pending', attempts = 0, not_before = now() WHERE status = 'failed';
```

## Loop live

Kandidat: laga Flashscore berstatus `live`, plus laga `scheduled` yang jam kick-off-nya sudah lewat (maks 3 jam) tapi halaman liga belum menangkapnya. Per laga: status, babak, waktu mulai babak, dan skor; kejadian ikut diambil kalau laga sudah mulai. Tanpa laga live, loop ini hanya satu query terindeks dan tidak ada request keluar.

Laga SofaScore tidak ikut loop live; skornya diperbarui oleh sync SofaScore per jam.

## Aturan data pemain

- **Klub pemain** (`players.team_id`) diambil dari sumber terbaru:
  - Lineup laga timnas tidak mengubah klub.
  - Lineup laga yang lebih lama dari laga lain yang sudah tersimpan untuk pemain itu tidak mengubah klub maupun nomor punggung. Jadi backfill atau halaman klub yang membawa hasil lama tidak memindahkan pemain kembali ke klub lamanya.
  - Skuad dan top players dianggap data terkini dan selalu menetapkan klub.
- **Nama** mengikuti tulisan lineup ("Husna A. M."); skuad tidak menimpanya.
- **Posisi** dari skuad (Flashscore) atau profil pemain (SofaScore); lineup Flashscore hanya mengisi `GK`.

## Mode sekali jalan

**`ingestor -backfill-lineups`** membaca ulang lineup tiap laga yang disimpan sebelum formasi, slot, dan rating ikut disimpan (ada starter tanpa `formation_place`), terbaru dulu, satu request per laga. Laga SofaScore butuh `SOFASCORE_PROXY` bila API-nya minta captcha; kalau tetap diblokir, laga SofaScore dilewati dan laga Flashscore jalan terus. Blokir Flashscore menghentikan backfill.

**`seed-sofascore`** untuk isi awal: semua kompetisi SofaScore, semua skuad, dan detail semua laga selesai sekaligus, tanpa batas per run milik ingestor. Setelah itu ingestor yang menjaga datanya tetap baru. Dengan `-images` hanya mengunduh logo dan foto yang belum ada, tanpa batas 100 foto per run (`img.sofascore.com` tidak pakai captcha).

## Menambah kompetisi

- **Flashscore**: baris di `competitions` dengan `flashscore_path` (mis. `/football/indonesia/liga-1/`), `active = true`, `scraped = true`. Run berikutnya mengambil halaman liganya; detail laga dan halaman klub menyusul lewat antrean.
- **SofaScore**: `INSERT INTO competitions (slug, name, country_id, type, sofascore_id, scraped) VALUES (..., false)`, lalu jalankan `seed-sofascore` untuk isi awal.

## Konfigurasi

| Env | Default | Keterangan |
|---|---|---|
| `INGEST_INTERVAL` | `5m` | Interval loop league, min `1m` |
| `INGEST_LIVE_INTERVAL` | `20s` | Interval loop live, min `10s` |
| `INGEST_DETAILS_PER_RUN` | `20` | Job per run; `0` mematikan antrean |
| `ASSETS_DIR` | `data/assets` | Folder gambar; harus sama dengan yang disajikan API |
| `FLASHSCORE_FSIGN` | bawaan | Ganti kalau feed Flashscore mulai menjawab 403, lihat [`files/07-scraping-flashscore.md`](files/07-scraping-flashscore.md) |
| `SOFASCORE_PROXY` | kosong | Browser yang lolos captcha (`scrap-claude/sofascore_proxy.py`), mis. `http://localhost:8787` |

## Diblokir sumber

| Gejala di log | Tindakan |
|---|---|
| `source blocked us, pausing jobs` | Antrean berhenti 15 menit lalu lanjut sendiri. Kalau terus berulang: turunkan `INGEST_DETAILS_PER_RUN` atau perbarui `FLASHSCORE_FSIGN` |
| `sync sofascore failed` dengan 403 / `challenge` | Jalankan `scrap-claude/sofascore_proxy.py` dan set `SOFASCORE_PROXY` |
| `show more pages failed` | Laga di luar 100 pertama tidak diperbarui; dicoba lagi run berikutnya |
| `scrape squad failed` | Tim tidak ditandai sudah sync, dicoba lagi run berikutnya |
