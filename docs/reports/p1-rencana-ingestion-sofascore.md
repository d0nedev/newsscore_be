# Rencana Integrasi Data Ingestion SofaScore (Kompetisi Indonesia)

**Tanggal:** 27 September 2026
**Terkait:** Perubahan Penyedia Data (dari SportMonks ke SofaScore)
**Fokus:** Semua Kompetisi Sepak Bola Indonesia Tahun Ini (Liga 1, Liga 2, Piala Presiden, Piala Indonesia, dll)

## 1. Latar Belakang & Sasaran
Kebutuhan untuk mengambil data dari SofaScore sebagai penyedia data utama/sekunder menggantikan atau melengkapi rancangan awal (SportMonks). Sofascore tidak menyediakan API publik resmi, sehingga pengambilan data harus dilakukan melalui endpoint internal (undocumented API) mereka dengan penanganan khusus terhadap `User-Agent`, header HTTP, dan potensi pemblokiran (Cloudflare/Rate Limiting).

**Sasaran:**
- Membangun klien API SofaScore di Go yang tangguh.
- Memetakan data dari SofaScore (Liga 1, Liga 2, dll) ke domain internal.
- Menjalankan penjadwalan ingestor bertingkat (live 20 detik, harian, dst) seperti pada rancangan arsitektur `03-backend-go.md`.

## 2. Tabel Item yang Direncanakan (Action Plan)

| ID | Tahap Pekerjaan | Deskripsi & Tantangan Utama | File / Modul yang Terdampak |
|---|---|---|---|
| 1 | **Riset Endpoint & Mapping Data** | Menemukan `unique_tournament_id` untuk kompetisi Indonesia. Memetakan JSON SofaScore (Events, Standings, Teams) ke domain aplikasi. | `docs/files/sofascore-mapping.md` (baru) |
| 2 | **Implementasi Klien HTTP SofaScore** | Membuat package `provider/sofascore` dengan HTTP Client khusus. Wajib menyertakan custom `User-Agent`, `Origin`, `Referer`, dan *circuit breaker* (Sony Gobreaker) agar sistem tidak hang bila hulu melambat. | `internal/provider/sofascore/client.go`<br>`internal/provider/sofascore/dto.go` |
| 3 | **Anti-Corruption Layer (ACL)** | Menerjemahkan DTO SofaScore menjadi entitas internal (`domain.Match`, `domain.MatchEvent`, `domain.Standing`). Penyeragaman status pertandingan (misal: "inprogress" -> `StatusLive`). | `internal/provider/sofascore/mapper.go` |
| 4 | **Pengembangan Ingestor Worker** | Membangun `internal/ingest` yang berisi penjadwal (scheduler) berjenjang: Live (20 detik), Harian (5 menit). Mengambil jadwal pertandingan kompetisi Indonesia hari ini, lalu masuk ke mode pemantauan live. | `internal/ingest/scheduler.go`<br>`internal/ingest/worker.go` |
| 5 | **State Diffing & Live Events** | Implementasi `internal/ingest/differ.go`. Menghitung selisih (diff) antara snapshot di DB dan data baru dari SofaScore (misal: penambahan gol, kartu merah) lalu menerbitkan event (SSE pubsub). Tangani kasus anomali (gol VAR dianulir). | `internal/ingest/differ.go` |
| 6 | **Penyimpanan ke Database (SQLC)** | Memastikan data yang ditarik (Tim, Pertandingan, Kejadian, Klasemen) masuk ke PostgreSQL lewat layer `repository`. Harus bersifat *idempotent* (Upsert) berdasarkan `external_id`. | `internal/repository/` |

## 3. Strategi Penanganan (Undocumented API) SofaScore
SofaScore menggunakan struktur endpoint seperti:
- **Daftar Kompetisi (Indonesia)**: Perlu mencari ID kategori/negara (Indonesia) dan ID Unik Turnamen (Unique Tournament ID). Contoh: Liga 1 Indonesia.
- **Jadwal Pertandingan (Fixtures)**: `https://api.sofascore.com/api/v1/sport/football/scheduled-events/{date}`
- **Detail Pertandingan & Insiden (Live)**: `https://api.sofascore.com/api/v1/event/{id}` dan `https://api.sofascore.com/api/v1/event/{id}/incidents`
- **Klasemen**: `https://api.sofascore.com/api/v1/unique-tournament/{id}/season/{season_id}/standings/total`

**Proteksi Anti-Bot:**
Kita akan mengandalkan Header HTTP yang disamarkan menyamai peramban (browser) umum, menerapkan jeda/backoff yang wajar untuk menjaga batas *rate limiting* yang tidak diketahui, serta menyimpan respons yang tidak berubah (*Not Modified* caching bila didukung header `ETag` atau `Last-Modified`).

## 4. Pengujian yang Dibutuhkan
1. **Mock Responses**: Menyimpan contoh JSON aktual dari SofaScore ke dalam direktori `testdata/` untuk keperluan *unit testing* dari *Mapper*.
2. **Differ Tests**: *Table-driven tests* untuk `differ.go` dengan skenario: (1) skor bertambah, (2) skor dianulir, (3) status berubah ke *finished*.

## 5. Pertanyaan & Batasan (Scope)
- **Batasan**: Kita hanya akan memantau kompetisi Indonesia (daftar ID kompetisi akan di-_hardcode_ atau diambil dari basis data melalui konfigurasi).
- **Out of Scope saat ini**: Sinkronisasi data historis masa lalu (tahun-tahun sebelumnya), kita hanya akan fokus ke data musim/tahun ini sesuai permintaan.
