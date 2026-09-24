# 06 — Rencana Pengerjaan

## 1. Fase

Total 13 pekan. Urutannya disusun agar bagian paling berisiko dikerjakan lebih
dulu, bukan agar terlihat ada kemajuan sejak awal.

### Fase 0 — Persiapan (1 pekan)

- Repositori, branching, konvensi commit, CI dasar
- Docker Compose untuk pengembangan lokal
- Skema basis data awal dan migrasi pertama
- Kaji SportMonks: endpoint mana yang tersedia pada paket langganan, batas
  kuota, dan seberapa dalam cakupan Liga 1 sebenarnya
- Definisikan spesifikasi OpenAPI untuk endpoint inti

**Selesai bila:** `docker compose up` menjalankan Postgres, Redis, dan dua
binary Go yang menjawab `/healthz`; dan hasil kajian SportMonks tertulis.

Fase ini tidak boleh dilewati. Kalau ternyata cakupan Liga 1 di SportMonks
lebih tipis dari dugaan, seluruh premis produk berubah dan lebih baik diketahui
sekarang daripada di pekan kedelapan.

### Fase 1 — Fondasi backend (2 pekan)

- Struktur proyek dan aturan ketergantungan
- Klien SportMonks beserta lapisan pemetaannya
- Repository dengan sqlc, seluruh entitas inti
- Lapisan cache dengan stale-while-revalidate dan single-flight
- Middleware: request id, logging, pemulihan panic, pembatasan laju
- Sentry dan log terstruktur

**Selesai bila:** data satu pertandingan dapat ditarik dari SportMonks,
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

### Fase 3 — Fondasi frontend dan halaman skor (2 pekan)

- Proyek Nuxt, token desain, tata letak dasar
- Komponen `MatchRow` beserta seluruh keadaannya
- Halaman `/pertandingan/[tanggal]` dengan SSR
- Navigasi tanggal, pengelompokan per kompetisi
- Mode terang dan gelap

**Selesai bila:** halaman pertandingan lolos anggaran performa pada Slow 4G
dengan data sungguhan, bukan data contoh.

### Fase 4 — Halaman detail (2 pekan)

- Halaman pertandingan: kronologi, susunan pemain, statistik, rekam pertemuan
- Klasemen dengan penanda zona
- Halaman klub dan pemain
- Pencarian

**Selesai bila:** seluruh halaman terhubung satu sama lain tanpa jalan buntu,
dan setiap halaman lolos pemeriksaan aksesibilitas otomatis.

### Fase 5 — Berita dan CMS (2 pekan)

- Tabel artikel dan relasi entitas
- API berita, publik dan admin
- Autentikasi admin dengan JWT
- Panel admin: menulis, menyunting, menerbitkan, menautkan ke entitas
- Halaman berita publik dan artikel
- Blok berita terkait pada halaman pertandingan dan klub

**Selesai bila:** sebuah artikel dapat ditulis di panel admin, ditautkan ke
sebuah pertandingan, dan muncul di halaman pertandingan itu tanpa deployment.

### Fase 6 — Pembaruan langsung (1 pekan)

- SSE hub dan publikasi lewat Redis
- Composable klien beserta penyambungan ulang
- Kilatan perubahan skor dan pengumuman `aria-live`
- Uji beban pada koneksi SSE

**Selesai bila:** seratus koneksi bersamaan dapat dilayani dengan pemakaian
memori yang stabil, dan gol muncul di layar dalam 25 detik sejak tercatat di
hulu.

### Fase 7 — SEO, performa, aksesibilitas (1 pekan)

- Data terstruktur seluruh jenis halaman
- Sitemap, robots, kanonis, hreflang
- Gambar Open Graph dinamis untuk pertandingan
- Audit Lighthouse dan perbaikan sampai seluruh anggaran terpenuhi
- Audit aksesibilitas manual dengan papan ketik dan pembaca layar

**Selesai bila:** seluruh angka pada tabel anggaran di dokumen frontend
terpenuhi, dan hasil uji data terstruktur bersih.

### Fase 8 — Pengerasan dan peluncuran (1 pekan)

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
| 3 — Fondasi frontend | 2 | 7 |
| 4 — Halaman detail | 2 | 9 |
| 5 — Berita dan CMS | 2 | 11 |
| 6 — Pembaruan langsung | 1 | 12 |
| 7 — SEO dan performa | 1 | 13 |
| 8 — Peluncuran | 1 | 14 |

Empat belas pekan bila dijalankan berurutan. Tiga belas pekan bila Fase 3
dimulai sebelum Fase 2 sepenuhnya selesai, yang memungkinkan setelah kontrak
API disepakati pada Fase 0.

## 3. Definition of done

Berlaku untuk setiap pull request, bukan hanya untuk fase:

- Uji lolos, dan cakupan tidak turun
- `golangci-lint` dan `eslint` bersih
- Perubahan skema disertai migrasi yang dapat dibalik
- Endpoint baru masuk ke spesifikasi OpenAPI
- Halaman baru tidak melanggar anggaran bundel
- Komponen baru dapat diakses dengan papan ketik
- Tidak ada nilai sensitif yang tertulis di dalam kode

## 4. Risiko

| Risiko | Dampak | Penanganan |
|---|---|---|
| Cakupan Liga 1 di SportMonks lebih tipis dari dugaan | Premis produk runtuh | Diverifikasi di Fase 0, sebelum ada kode yang ditulis |
| Kuota SportMonks terlampaui | Layanan berhenti | Single-flight dan cache sejak Fase 1, pantau pemakaian sejak hari pertama |
| Differ menghasilkan event berlipat | Data salah dan kepercayaan hilang | Kunci unik di basis data, uji tabel rapat, uji 48 jam di Fase 2 |
| Anggaran performa terlampaui di akhir | Perbaikan mahal | Diukur sejak Fase 3, bukan diserahkan ke Fase 7 |
| Berita tidak terisi setelah peluncuran | Pembeda utama hilang | Sepakati sejak awal siapa yang menulis dan berapa artikel per pekan |
| Satu VPS tidak cukup saat pertandingan besar | Situs tumbang di momen paling ramai | Uji beban di Fase 8, siapkan rencana penskalaan sebelum peluncuran |

Risiko kelima adalah yang paling sering terwujud dan paling sering diabaikan.
Seluruh arsitektur di dokumen ini bertumpu pada berita yang menempel ke
pertandingan. Kalau tidak ada yang menulis beritanya, yang tersisa hanyalah
situs skor biasa yang bersaing langsung dengan SofaScore — pertarungan yang
sudah disimpulkan tidak bisa dimenangkan. Kejelasan soal siapa yang mengisi
konten harus didapat sebelum Fase 5 dimulai, sebaiknya sebelum proyek berjalan.

## 5. Hal yang perlu diputuskan sebelum mulai

1. Siapa penulis beritanya, dan berapa artikel per pekan yang realistis
2. Paket SportMonks yang aktif dan batas kuotanya
3. Nama domain dan identitas merek
4. Apakah panel admin cukup untuk satu penulis, atau perlu banyak pengguna
   dengan peran berbeda
5. Apakah anggaran klien sudah disesuaikan dengan lingkup yang jauh lebih
   besar daripada QUO-2026-002
