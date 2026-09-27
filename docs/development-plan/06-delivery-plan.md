# 06 — Rencana Pengerjaan

## 1. Fase

Total 9 pekan (backend saja; frontend di luar dokumen ini). Urutannya disusun agar bagian paling berisiko dikerjakan lebih
dulu, bukan agar terlihat ada kemajuan sejak awal.

### Fase 0 — Persiapan (1 pekan)

- Repositori, branching, konvensi commit, CI dasar
- Docker Compose untuk pengembangan lokal
- Skema basis data awal dan migrasi pertama
- Kaji Flashscore: feed mana yang stabil, seberapa dalam cakupan Liga 1
  (events, stats, lineups), ambang blokir, dan status hukum scraping
- Definisikan spesifikasi OpenAPI untuk endpoint inti

**Selesai bila:** `docker compose up` menjalankan Postgres, Redis, dan dua
binary Go yang menjawab `/healthz`; dan hasil kajian Flashscore tertulis.

Fase ini tidak boleh dilewati. Kalau ternyata cakupan Liga 1 di Flashscore
lebih tipis, atau scraping tidak layak secara hukum, dari dugaan, seluruh premis produk berubah dan lebih baik diketahui
sekarang daripada di pekan keenam.

### Fase 1 — Fondasi backend (2 pekan)

- Struktur proyek dan aturan ketergantungan
- Scraper Flashscore, parser `¬~ ¬ ÷`, dan pemetaan ke domain
- Repository dengan sqlc, seluruh entitas inti
- Lapisan cache dengan stale-while-revalidate dan single-flight
- Autentikasi sesi cookie HttpOnly (tabel `sessions`)
- Middleware: request id, logging, pemulihan panic, pembatasan laju
- Sentry dan log terstruktur

**Selesai bila:** data satu pertandingan dapat ditarik dari Flashscore,
tersimpan di PostgreSQL, dan terbaca kembali lewat endpoint HTTP.

### Fase 2 — Ingestion dan API inti (2 pekan)

- Scheduler berjenjang
- Differ dan deteksi event, dengan uji tabel yang rapat
- Circuit breaker dan batas waktu terhadap hulu
- Endpoint: pertandingan per tanggal, detail pertandingan, klasemen, klub,
  pemain
- Pencarian dengan `pg_trgm`

**Selesai bila:** ingestor berjalan tanpa henti selama 48 jam melewati
setidaknya satu hari pertandingan, tanpa event berlipat dan tanpa kebocoran
memori.

Fase ini adalah puncak risiko teknis proyek. Kalau ada fase yang molor, paling
mungkin fase ini.

### Fase 3 — Berita dan API CMS (2 pekan)

- Tabel artikel dan relasi entitas
- API berita, publik dan admin
- Peran admin di atas sesi cookie
- Endpoint admin: menulis, menyunting, menerbitkan, menautkan ke entitas
- Endpoint berita terkait per pertandingan dan klub

**Selesai bila:** sebuah artikel dapat dibuat lewat API admin, ditautkan ke
sebuah pertandingan, dan muncul di respons endpoint berita terkait pertandingan itu.

### Fase 4 — Pembaruan langsung (1 pekan)

- SSE hub dan publikasi lewat PostgreSQL `LISTEN/NOTIFY` (trigger di `matches` dan `match_events`)
- Dukungan `Last-Event-ID` untuk penyambungan ulang klien
- Uji beban pada koneksi SSE

**Selesai bila:** seratus koneksi bersamaan dapat dilayani dengan pemakaian
memori yang stabil, dan event gol terkirim lewat SSE dalam 25 detik sejak tercatat di
hulu.

### Fase 5 — Pengerasan dan peluncuran (1 pekan)

- Pencadangan basis data dan uji pemulihan yang benar-benar dijalankan
- Peringatan otomatis dan dasbor pemantauan
- Peninjauan keamanan: pembatasan laju, kebijakan CORS, header keamanan,
  pemeriksaan kebocoran kunci
- Pengujian beban pada skenario hari pertandingan
- Deployment production dan pemantauan intensif selama 72 jam

**Selesai bila:** pemulihan dari cadangan pernah dijalankan sampai berhasil,
dan seluruh peringatan pernah dipicu secara sengaja untuk memastikan sampai.

## 2. Ringkasan jadwal

| Fase | Pekan | Kumulatif |
|---|---|---|
| 0 — Persiapan | 1 | 1 |
| 1 — Fondasi backend | 2 | 3 |
| 2 — Ingestion dan API | 2 | 5 |
| 3 — Berita dan API CMS | 2 | 7 |
| 4 — Pembaruan langsung | 1 | 8 |
| 5 — Peluncuran | 1 | 9 |

Sembilan pekan bila berurutan. Frontend dapat berjalan paralel setelah
kontrak API disepakati di Fase 0.

## 3. Definition of done

Berlaku untuk setiap pull request, bukan hanya untuk fase:

- Uji lolos, dan cakupan tidak turun
- `golangci-lint` bersih
- Perubahan skema disertai migrasi yang dapat dibalik
- Endpoint baru masuk ke spesifikasi OpenAPI
- Tidak ada nilai sensitif yang tertulis di dalam kode

## 4. Risiko

| Risiko | Dampak | Penanganan |
|---|---|---|
| Cakupan Liga 1 di Flashscore lebih tipis dari dugaan | Premis produk runtuh | Diverifikasi di Fase 0, sebelum ada kode yang ditulis |
| IP server diblokir Cloudflare | Data berhenti | Hanya ingestor yang scrape, jeda 1 detik, detail hanya saat status berubah, cache + single-flight |
| Format feed atau `x-fsign` berubah | Parser gagal diam-diam | Testdata rekaman, peringatan saat 403/kosong/gagal parse, penyedia cadangan `sofascore` |
| Scraping melanggar ketentuan layanan | Tuntutan hukum, situs ditutup | Kaji di Fase 0; siapkan jalur migrasi ke penyedia berlisensi |
| Differ menghasilkan event berlipat | Data salah dan kepercayaan hilang | Kunci unik di basis data, uji tabel rapat, uji 48 jam di Fase 2 |
| Latensi API p95 terlampaui | Frontend lambat | Ukur p95 sejak Fase 2 |
| Berita tidak terisi setelah peluncuran | Pembeda utama hilang | Sepakati sejak awal siapa yang menulis dan berapa artikel per pekan |
| Satu VPS tidak cukup saat pertandingan besar | Situs tumbang di momen paling ramai | Uji beban di Fase 5, siapkan rencana penskalaan sebelum peluncuran |

Risiko kelima adalah yang paling sering terwujud dan paling sering diabaikan.
Seluruh arsitektur di dokumen ini bertumpu pada berita yang menempel ke
pertandingan. Kalau tidak ada yang menulis beritanya, yang tersisa hanyalah
situs skor biasa yang bersaing langsung dengan SofaScore — pertarungan yang
sudah disimpulkan tidak bisa dimenangkan. Kejelasan soal siapa yang mengisi
konten harus didapat sebelum Fase 3 dimulai, sebaiknya sebelum proyek berjalan.

## 5. Hal yang perlu diputuskan sebelum mulai

1. Siapa penulis beritanya, dan berapa artikel per pekan yang realistis
2. Apakah scraping Flashscore diterima secara hukum, atau perlu anggaran
   penyedia berlisensi
3. Nama domain dan identitas merek
4. Apakah panel admin cukup untuk satu penulis, atau perlu banyak pengguna
   dengan peran berbeda
5. Apakah anggaran klien sudah disesuaikan dengan lingkup yang jauh lebih
   besar daripada QUO-2026-002
