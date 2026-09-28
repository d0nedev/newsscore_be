# Rencana Ingestion Multi-Provider (Flashscore + SofaScore)

**Tanggal:** 28 September 2026
**Terkait:** [`03-backend-go.md`](03-backend-go.md), [`postgres-schema-plan.md`](postgres-schema-plan.md), [`../files/07-scraping-flashscore.md`](../files/07-scraping-flashscore.md), [`../reports/p1-rencana-ingestion-sofascore.md`](../reports/p1-rencana-ingestion-sofascore.md)

## Cakupan

| Provider | Kompetisi | Data |
|---|---|---|
| Flashscore | Super League (Liga 1), Championship | Hasil, jadwal, klasemen, event, statistik, lineup, pemain |
| SofaScore | Kompetisi Indonesia lainnya: Liga Nusantara, Liga 4, Piala Presiden, Piala Indonesia, EPA (U-20/U-18/U-16), Liga 1 Putri, dll. | Hasil, jadwal, klasemen/grup, event, statistik, lineup |

Setiap kompetisi punya satu provider **primer**. Provider lain boleh ikut sebagai **sekunder** untuk failover dan validasi silang (lihat bagian 6). Pertandingan dari provider sekunder tidak pernah dibuat sebagai baris baru; ia hanya dipetakan ke pertandingan kanonik yang sudah ada.

## 1. Masalah di kode sekarang yang harus dibereskan dulu

| # | Temuan | Dampak |
|---|---|---|
| 1 | Skema terikat ke satu provider: kolom `teams.flashscore_id` dan `matches.flashscore_id` bersifat `UNIQUE NOT NULL` | Data SofaScore tidak bisa masuk. Klub yang sama (misalnya Persib di Piala Indonesia) akan tercatat dua kali |
| 2 | Primary key `standings` adalah `(season, team_id)`, tanpa kompetisi | Klasemen Liga 1, Championship, dan piala akan saling menimpa |
| 3 | `Season: 2026` di-hardcode, dan URL scraper di-hardcode ke `super-league/results` | Belum bisa menangani banyak liga atau banyak musim |
| 4 | Status dihitung dengan `AB != "3"`, jadi apa pun selain "3" dianggap `scheduled` | Pertandingan live, ditunda, atau dibatalkan salah status |
| 5 | Klasemen dihitung sendiri dan hanya berisi poin. W/D/L/GF/GA tidak disimpan dan tidak ada tie-breaker | Peringkat bisa salah |
| 6 | `initialFeeds['results']` hanya memuat sekitar 1 halaman terakhir (±28 pertandingan) | Satu musim penuh tidak ikut tertarik |
| 7 | `ScrapeMatchDetails` tidak dipanggil di mana pun, dan worker tidak di-wire ke `main.go` | Detail pertandingan dan penjadwalan belum jalan |
| 8 | `sample_sofascore_liga1.json` berisi data palsu (ID tim 11111, 22222) | Belum ada bukti endpoint SofaScore bisa diakses dari server |

## 2. Fase pekerjaan

### Fase 0: Spike dan riset (lakukan sebelum menulis kode)

| ID | Pekerjaan | Output |
|---|---|---|
| 0.1 | Verifikasi slug Flashscore untuk Championship (cek apakah `indonesia/championship/` atau `indonesia/liga-2/`) dan untuk halaman `fixtures/` dan `standings/` | Tabel URL |
| 0.2 | Reverse-engineer pagination "Show more" di Flashscore (feed `x/feed/...` dengan `x-fsign`) dan feed klasemen | Catatan ditambahkan ke `docs/files/07-scraping-flashscore.md` |
| 0.3 | Uji akses SofaScore dari IP server: `www.sofascore.com/api/v1/...` dengan header browser. Kalau kena 403 Cloudflare, uji TLS impersonation (`bogdanfinn/tls-client`) | **Selesai (28 Sep 2026)**: API 403 untuk semua cara; halaman HTML bisa. Lihat bagian 8 |
| 0.4 | Discovery kompetisi Indonesia: `GET /api/v1/category/{id_indonesia}/unique-tournaments`, lalu catat `unique_tournament_id` dan `season_id` tiap kompetisi | **Sebagian selesai**: 10 turnamen Indonesia ditemukan lewat sitemap. Lihat bagian 8 |

### Fase 1: Skema master data, staging provider, dan routing (migrasi `0005`)

Alur data mengikuti bagian 7: **fetch → staging `provider_*` → matching → sync ke tabel kanonik**.

- **Master data kanonik (didefinisikan dulu, lihat 7.2):**
  - `countries (code ISO 3166, name)`: seed statis
  - `competitions (id, slug, name, country_code, gender, age_group, type league|cup, tier)`: seed manual untuk kompetisi prioritas
  - `seasons (id, competition_id, name "2026/27", start_date, end_date, is_current)`
  - `stat_types (key, name_id, unit)` dan `stat_provider_keys (stat_key, provider, raw_key)`: kamus statistik manual
  - `team_aliases` dan kamus sinonim nama kompetisi ("Liga 1" = "Super League")
- **Pemetaan dan routing:**
  - `external_refs (provider, entity_type, external_id, internal_id, method, confidence, UNIQUE(provider, entity_type, external_id))`, berlaku untuk country, kompetisi, musim, tim, pertandingan, dan pemain. `method` (`manual`, `alias`, `similarity`, `fixture`) dan `confidence` mencatat cara pemetaan dibuat (lihat 7.4)
  - `match_candidates (provider, entity_type, external_id, candidate_internal_id, score, status pending|accepted|rejected)`: review queue
  - `provider_routes (competition_id nullable = default, data_type, provider, priority, enabled)`: urutan fallback per kompetisi dan jenis data (lihat 7.3). ID tidak di-hardcode
- **Staging per provider (data mentah):**
  - `provider_competitions`, `provider_teams`, `provider_players`: katalog hasil discovery
  - `provider_matches`, `provider_match_blocks (provider, external_match_id, block_type score_status|events|statistics|lineups, payload JSONB, payload_hash, fetched_at)`
  - Semua baris punya `provider`, `external_id`, `payload_hash`, `fetched_at`. Data yang belum ter-link tetap disimpan
- **Perubahan tabel kanonik lama:**
  - `matches`: tambah `season_id` (FK) dan `round`; `flashscore_id` pindah ke `external_refs`
  - `standings`: PK menjadi `(season_id, team_id)`, tambah `played, won, drawn, lost, gf, ga, gd, group_name`
  - `match_events` dan `players`: `flashscore_id` pindah ke `external_refs`; `players` tambah `birth_date`
  - Blok kanonik (skor/status, event, statistik, lineup) menyimpan `source_provider` dan `data_as_of`
- **Query sqlc baru:** `ResolveExternalRef`, `UpsertProviderBlock`, `ListRoutes`, `UpsertCompetition`, `UpsertSeason`, `UpsertStanding` (versi lengkap), `ListMatchesNeedingDetails`, `ListPendingCandidates`.

### Fase 2: Abstraksi provider dan Flashscore lengkap

- Definisikan kontrak di `internal/provider/provider.go`:

  ```go
  type Fetcher interface {
      Name() string
      Supports(dt DataType) bool
      // Catalog: discovery kompetisi, tim, pemain (tingkat katalog, lihat 7.1)
      Catalog(ctx context.Context, kind EntityType, scope ExternalRef) ([]CatalogItem, error)
      // Fetch: isi (jadwal, skor, event, statistik, lineup) untuk entitas yang sudah ter-link
      Fetch(ctx context.Context, dt DataType, ref ExternalRef) (Block, error)
  }
  ```

  Fetcher hanya menulis ke staging `provider_*`; ia tidak tahu soal tabel kanonik.
- Buat `internal/provider/httpclient`, klien bersama untuk kedua provider. Isinya: timeout, rate limiter per host (`x/time/rate`), retry dengan exponential backoff dan jitter, circuit breaker, dan metrik Prometheus (`provider_requests_total{provider,status}`). Ini menggantikan `time.Sleep(1s)` dan `http.DefaultClient`.
- Di Flashscore:
  - URL dan `x-fsign` dipindah ke config/env
  - Parser untuk halaman `results` (dengan pagination), `fixtures`, dan `standings`
  - Pemetaan status lengkap (scheduled, live, HT, finished, postponed, cancelled, abandoned)
  - Detail pertandingan (event, statistik, lineup) disambungkan ke DB
  - Satu parser `¬~ ¬ ÷` yang dipakai bersama; saat ini logikanya terduplikasi di `scraper.go` dan `details.go`
- Unit test parser dengan fixture feed asli di `internal/provider/flashscore/testdata/`.
- Buat `internal/ingest/router.go`: memilih provider sesuai `provider_routes` dan menjalankan fallback (lihat 7.3).

### Fase 3: Provider SofaScore (kompetisi Indonesia selain Liga 1 dan Championship)

- **Catatan hasil spike (bagian 8):** API `/api/v1` terblokir, jadi rancangan endpoint di bawah belum bisa dipakai. Yang tersedia hanya data dari HTML (`__NEXT_DATA__`) dan sitemap. Fase ini perlu dirancang ulang setelah keputusan di 8.5.
- File yang dibuat di `internal/provider/sofascore/`:
  - `client.go`, memakai header browser dan TLS client sesuai hasil 0.3
  - `dto.go`
  - `mapper.go`, anti-corruption layer ke domain, termasuk pemetaan `status.type` (`notstarted`, `inprogress`, `finished`, `postponed`, `canceled`)
- Endpoint yang dipakai:
  - `unique-tournament/{ut}/seasons`: menentukan musim aktif
  - `unique-tournament/{ut}/season/{s}/events/last/{page}` dan `.../next/{page}`: semua pertandingan satu musim (dengan pagination)
  - `unique-tournament/{ut}/season/{s}/standings/total`: untuk liga dan fase grup piala
  - `event/{id}/incidents`, `/statistics`, `/lineups`: detail pertandingan
- Format piala perlu penanganan khusus: pertandingan knockout tidak punya klasemen, sehingga `round` dan `group_name` wajib diisi.
- Aturan anti-duplikasi: `provider_routes.priority` menentukan provider primer per kompetisi dan jenis data. Untuk Liga 1 dan Championship, Flashscore primer dan SofaScore sekunder. Data sekunder hanya dipetakan ke entitas kanonik lewat reconciler (bagian 6).
- Hapus `mapper_sample.go` dan JSON palsunya, lalu ganti dengan test yang memakai `testdata` asli.

### Fase 4: Orkestrasi ingestion (discovery, fetch, matching, sync)

- `internal/ingest/discovery.go`: tarik katalog dari semua provider ke `provider_competitions/teams/players`, lalu jalankan matcher.
- `internal/ingest/matcher.go`: filter konteks + similarity (lihat 7.4). Hasilnya auto-link ke `external_refs` atau masuk `match_candidates`.
- `internal/ingest/router.go`: fetch isi lewat `provider_routes` dengan fallback, tulis ke `provider_match_blocks`.
- `internal/ingest/reconciler.go`: baca staging, pilih blok terbaik per pertandingan (aturan 6.6), upsert ke tabel kanonik dalam satu transaksi per pertandingan.
- Mode job:
  1. **Discovery katalog**: mingguan, dan manual saat menambah kompetisi
  2. **Backfill** musim berjalan: dijalankan sekali atau manual lewat `cmd/ingest --backfill`, hanya untuk entitas yang sudah ter-link
  3. **Harian** (misalnya 03:00 WIB): jadwal, hasil, dan klasemen, lalu sync batch
  4. **Match-day**: setiap 5 menit (atau lebih rapat) dalam jendela kick-off −15 menit sampai +150 menit. Reconciler berjalan **langsung per pertandingan** begitu `payload_hash` berubah, tidak menunggu batch
  5. **Detail**: pertandingan yang baru berstatus `finished` dan belum punya detail. Jangan loop ulang seluruh riwayat
  6. **Re-sync**: setelah link di review queue disetujui, reconciler memproses ulang data dari staging tanpa fetch ulang
- Scheduler (`robfig/cron` atau ticker) dijalankan dari `cmd/server` atau binary terpisah `cmd/ingestor`, lengkap dengan `pg_try_advisory_lock` supaya job tidak jalan dobel saat ada banyak replika.
- Endpoint admin untuk review queue: daftar kandidat, terima, tolak.
- Tambah struct `IngestConfig` di `config.go`: `INGEST_ENABLED`, `FLASHSCORE_FSIGN`, rate per provider, cron expression, ambang similarity.
- Ganti `log.Printf` dengan `slog`, dan setiap run dicatat ke tabel `ingest_runs` (provider, jenis job, jumlah upsert, error, durasi).

### Fase 5: Observability dan pengujian

- Metrik Prometheus: `ingest_run_duration_seconds`, `ingest_items_upserted_total`, `ingest_last_success_timestamp{source}`.
- Alert di `prometheus-alerts.yml`, misalnya kalau tidak ada run yang sukses lebih dari 26 jam, atau error rate provider di atas ambang yang disepakati.
- Test:
  - Parser dan mapper: table-driven dengan fixture asli
  - Upsert idempoten: integration test Postgres, mengikuti pola `postgres_integration_test.go`
  - Resolusi tim lintas provider dan matcher (ambang, filter konteks, kasus "Liga 1" Indonesia vs negara lain)
  - Router: urutan fallback, data kosong, circuit breaker terbuka, `external_ref` belum ada
  - Reconciler: blok tidak tercampur antar provider, re-sync setelah link disetujui
- Metrik tambahan: `ingest_fallback_total{provider,data_type}`, `ingest_missing_ref_total{provider}`, `match_candidates_pending`.

## 3. Urutan eksekusi

`0 (spike) → 1 (skema) → 2 (Flashscore + router) → 4 (discovery, matcher, reconciler; hanya Flashscore) → 3 (SofaScore) → 5`

SofaScore sengaja dikerjakan setelah runner jalan, supaya provider kedua tinggal ditambahkan tanpa mengubah alur yang sudah ada.

## 4. Risiko

- **Anti-bot:** kedua sumber tidak resmi. `x-fsign` bisa berubah, dan SofaScore berada di belakang Cloudflare. Mitigasinya:
  - Konfigurasi bisa diganti tanpa deploy
  - Alert saat terjadi 403
  - Rate limit yang konservatif
- **ToS:** scraping kedua situs kemungkinan melanggar ketentuan layanan mereka. Ini perlu diperhitungkan kalau produknya komersial.
- **Nama kompetisi berubah-ubah:** misalnya Liga 1 menjadi Super League dan Liga 2 menjadi Championship. Karena itu `competitions.slug` internal harus stabil dan tidak bergantung pada nama dari provider.

## 5. Perkiraan volume data Flashscore

Angka di bagian ini **perkiraan kasar, belum diukur**. Ganti dengan angka nyata setelah spike pengukuran 1 hari (lihat 5.4).

### 5.1 Asumsi volume (seluruh sepak bola di Flashscore)

| Item | Perkiraan |
|---|---|
| Kompetisi sepak bola | ~1.000–1.500 (liga, piala, youth, putri, dari ±150 negara) |
| Pertandingan per hari | ~500–1.500, akhir pekan bisa lebih dari 2.000 |
| Pertandingan per tahun | ~250–400 ribu (dipakai ±300 ribu) |
| Pertandingan yang punya detail lengkap | ~30–40%. Liga kecil biasanya hanya punya skor dan gol |

### 5.2 Ukuran

| Data | Per musim |
|---|---|
| Database (pertandingan, detail, tim, pemain, klasemen, termasuk index) | ~1,5–2 GB. Untuk 10 musim historis ~10–15 GB |
| Unduhan mentah (halaman daftar + feed detail) | ~7–10 GB (~1 GB kalau disimpan dalam bentuk gzip) |

### 5.3 Hambatan utama: jumlah request

- Detail butuh 3 request per pertandingan, sehingga ~900 ribu request per musim.
- Pada rate aman 1 request/detik, backfill satu musim penuh makan waktu ~10–11 hari tanpa henti.
- Sync harian jauh lebih ringan: ~1 jam per hari.

### 5.4 Keputusan

1. Jalankan spike pengukuran: ambil feed pertandingan sepak bola untuk 1 hari (±10–20 request) untuk mendapatkan jumlah pertandingan, ukuran feed, dan persentase yang punya detail.
2. Batasi cakupan, misalnya Indonesia plus ±50 liga top dunia. Volume turun lebih dari 90%.
3. Detail (event, statistik, lineup) hanya untuk kompetisi prioritas. Kompetisi lain cukup skor dan status.

## 6. Provider alternatif dan sinkronisasi antar provider

### 6.0 Penjelasan sederhana

Bayangkan kita ingin tahu skor pertandingan, tapi tidak menonton sendiri. Jadi kita **bertanya ke beberapa teman** (provider).

- **Pilih teman yang sumbernya berbeda.** Soccerway kemungkinan "kembaran" Flashscore: kalau Flashscore salah, dia ikut salah.
- **Samakan nama.** "Persib Bandung" di satu teman dan "Persib" di teman lain dicatat sebagai tim yang sama di buku kita. Trik: kalau Persib main Sabtu jam 7 malam di catatan kita dan teman lain bilang "Persib lawan Tim X Sabtu jam 7 malam", berarti Tim X sama dengan lawan Persib di catatan kita.
- **Kalau cerita beda:** skor percaya teman utama dulu; daftar gol ambil dari satu teman saja supaya tidak terhitung dua kali; kalau ada tiga teman, pakai suara terbanyak; pertandingan yang sudah selesai tidak boleh mundur jadi "sedang main" tanpa konfirmasi.
- **Kalau teman utama sakit** (diblokir), pindah ke teman lain, tapi jangan bolak-balik tiap menit.
- **Tulis dulu, putuskan belakangan.** Semua jawaban dicatat apa adanya; satu "hakim" (reconciler) yang memutuskan mana yang masuk buku utama.

### 6.1 Provider alternatif

Belum diverifikasi ulang. Cek dulu sebelum dipakai.

| Provider | Akses | Liga Indonesia | Catatan |
|---|---|---|---|
| Soccerway | Scraping | Ada | Kemungkinan sudah berjalan di platform Livesport (induk Flashscore). Kalau benar, datanya sumber yang sama dengan Flashscore dan tidak berguna sebagai cadangan |
| LiveScore | API internal JSON (tidak resmi) | Ada, terutama Liga 1 | Kuat untuk skor live, detailnya tipis. Cocok sebagai cadangan skor dan status |
| FotMob | API internal JSON (tidak resmi) | Liga 1 ada | Statistik kaya. Ada proteksi header token |
| SofaScore | API internal JSON (tidak resmi) | Paling lengkap | Sudah masuk rencana (Fase 3) |
| API-Football (api-sports) | API resmi berbayar, ada tier gratis | Liga 1 dan Liga 2 ada | Satu-satunya pilihan yang legal dan stabil. Kandidat jaring pengaman produksi |
| Sportmonks | API resmi berbayar | Ada | Rencana awal proyek |
| Transfermarkt | Scraping | Ada | Bukan untuk skor. Untuk profil pemain, nilai pasar, transfer |

Prinsip memilih cadangan: **independensi sumber**, bukan jumlah provider. Kombinasi yang disarankan: Flashscore primer; SofaScore dan LiveScore sekunder; API-Football sebagai jaring pengaman legal.

### 6.2 Bentuk sinkronisasi

| Bentuk | Contoh | Kesulitan |
|---|---|---|
| Pemetaan entitas | Persib di Flashscore dan SofaScore menjadi satu `teams.id` | Sedang, fondasi untuk bentuk lainnya |
| Failover | Flashscore kena 403, Liga 1 sementara diambil dari SofaScore | Sedang |
| Enrichment | Skor dari Flashscore, statistik dari SofaScore | Sedang sampai tinggi |
| Validasi silang | Bandingkan skor/status kedua provider, alert kalau berbeda | Rendah, kalau pemetaan entitas sudah jadi |

### 6.3 Arsitektur: pisahkan ingest dari rekonsiliasi

```
Flashscore ─┐                               ┌─► tabel kanonik (teams, matches, events…)
SofaScore  ─┼─► ACL per provider ─► staging ─► Reconciler ─┤
LiveScore  ─┘   (DTO → domain)    (per provider,           └─► review queue (konflik/unmatched)
                                   snapshot mentah)
```

- Staging per provider (`provider_matches`, `provider_match_blocks`, dst.; skema di Fase 1) menyimpan data apa adanya beserta `fetched_at` dan hash payload.
- Reconciler membaca staging dan memutuskan nilai kanonik. Kalau aturan merge berubah, data kanonik dihitung ulang tanpa scraping lagi.
- Hash payload dipakai untuk melewati data yang tidak berubah.

### 6.4 Entity resolution berlapis

Urutkan dari yang paling pasti:

1. Crosswalk yang sudah ada di `external_refs`.
2. Alias manual di `team_aliases`, diisi sekali untuk tim prioritas.
3. Nama yang dinormalisasi + negara + kompetisi (huruf kecil, buang awalan "PS", "FC", "Persatuan", buang tanda baca). Jangan fuzzy match otomatis: "PSS Sleman" dan "PSIS Semarang" bisa tertukar.
4. **Pencocokan berbasis jadwal**: kompetisi sama, kick-off sama (± toleransi), dan satu tim sudah terpetakan, maka lawannya hampir pasti tim yang sama. Satu musim bisa terpetakan otomatis dari beberapa alias awal.
5. Tidak cocok: masuk review queue. Jangan membuat tim baru otomatis untuk kompetisi prioritas.

`external_refs.method` dan `confidence` disimpan supaya pemetaan lemah bisa diaudit.

### 6.5 Pencocokan pertandingan

```
(competition_id, home_team_id, away_team_id, |kickoff_a − kickoff_b| ≤ 36 jam)
```

Ada dua kandidat (misalnya leg 1 dan leg 2): pilih selisih waktu terkecil. Kandang/tandang tertukar: masuk review, jangan ditukar otomatis.

### 6.6 Aturan merge per blok

Gabungkan data **per blok utuh**, jangan mencampur isi satu blok dari beberapa provider.

| Blok | Aturan |
|---|---|
| Skor dan status | Prioritas provider per kompetisi. Pindah ke provider berikutnya kalau data primer basi (misalnya lebih dari 10 menit saat live) |
| Event (gol, kartu) | Seluruh daftar event satu pertandingan dari **satu** provider. Event tidak punya ID bersama, jadi menggabungkannya hampir pasti menghasilkan gol dobel |
| Statistik dan lineup | Provider pertama yang punya blok itu, sesuai prioritas |
| Jadwal (kick-off) | Provider primer. Flag kalau selisih dengan provider lain lebih dari 1 jam |

Setiap blok kanonik menyimpan `source_provider` dan `data_as_of`.

### 6.7 Penanganan konflik

- **Kuorum**: dengan 3 provider, skor ditentukan mayoritas (2 dari 3). Dengan 2 provider, pakai primer dan beri flag kalau berbeda.
- **Status sebagai state machine**: hanya maju (`scheduled → live → finished`). Mundur dianggap koreksi dan harus dikonfirmasi provider kedua.
- **Koreksi skor (VAR)**: skor yang turun diterima kalau stabil selama 2 polling berturut-turut, untuk menghindari kedip skor.

### 6.8 Failover berbasis kesehatan provider

- Hitung `success_rate` dan `freshness` per provider per kompetisi; circuit breaker per provider.
- Histeresis: failover setelah N kali gagal berturut-turut, kembali ke primer setelah M kali sukses berturut-turut.
- Setiap failover memancarkan metrik dan alert.

### 6.9 Tahapan

1. **MVP**: crosswalk + alias + pencocokan berbasis jadwal, merge per blok berdasarkan prioritas, alert kalau skor berbeda.
2. **Berikutnya**: tabel staging per provider + reconciler, failover dengan histeresis.
3. **Kalau ada provider ketiga**: kuorum.

## 7. Master data, routing, fallback, dan similarity

### 7.1 Alur besar

```
[1] Master data kanonik (kita yang definisikan)
    country → competition → season → team → player, + katalog statistik
          │
[2] Discovery katalog per provider (mingguan)
    daftar kompetisi/tim/pemain → provider_competitions/teams/players
          │
[3] Matching: filter konteks + similarity → skor keyakinan
    ≥ ambang tinggi → auto-link ke external_refs
    di tengah       → review queue (match_candidates)
    rendah          → abaikan
          │
[4] Routing: (kompetisi, jenis data) → daftar provider berurutan
          │
[5] Fetch isi pakai external ID → gagal/kosong? → fallback ke provider berikutnya
    hasil → provider_match_blocks (staging)
          │
[6] Reconciler: staging → tabel kanonik
```

Prinsip:

- **Similarity dipakai sekali saat matching, bukan setiap fetch.** Setelah link tersimpan di `external_refs`, semua fetch memakai ID.
- **"Semua data" dibagi dua tingkat:**

  | Tingkat | Yang di-fetch | Frekuensi |
  |---|---|---|
  | Katalog | Daftar kompetisi, musim, tim, pemain dari semua provider | Jarang (mingguan), ringan |
  | Isi | Jadwal, skor, event, statistik, lineup | Hanya untuk entitas yang sudah ter-link dan ada di `provider_routes` |

  Fetch isi untuk semua hal lalu mencocokkan belakangan akan menghabiskan ratusan ribu request untuk data yang mungkin tidak dipakai (bagian 5).
- **Sync live berjalan per pertandingan**, langsung saat `payload_hash` berubah. Sync batch hanya untuk data harian.
- **Data yang belum ter-link tetap disimpan** di staging. Setelah link disetujui, reconciler memproses ulang dari staging tanpa fetch ulang.
- **Fallback terjadi di langkah fetch.** Hasilnya tetap masuk ke staging milik provider yang menjawab. Reconciler memilih blok terbaik berdasarkan prioritas dan kesegaran.

### 7.2 Master data

| Entitas | Cara mendefinisikan | Cara mencocokkan ke provider |
|---|---|---|
| Country | Seed statis kode ISO 3166 (`ID`, `GB-ENG`, …) | Mapping manual, tanpa similarity |
| Competition | Seed manual: slug, nama, negara, gender, kelompok umur, tier, tipe | Similarity + filter konteks. **Kompetisi prioritas di-link manual** |
| Season | Diturunkan dari kompetisi (`2026/27`, tanggal) | Lewat tahun/tanggal, bukan nama |
| Team | Seed untuk kompetisi prioritas, sisanya dari provider primer | Similarity + negara + kompetisi + pencocokan berbasis jadwal |
| Player | Dari provider primer | Similarity nama + **tanggal lahir** + kewarganegaraan + tim + nomor punggung |
| Stat type | Katalog tetap (`possession`, `shots_on_target`, `xg`, …) + satuan | **Kamus manual** per provider. Jangan similarity: "Shots" dan "Shots on target" terlalu mirip |

Contoh kamus statistik:

```
stat_types:         ('shots_on_target', 'Tembakan ke gawang', 'count')
stat_provider_keys: ('shots_on_target', 'flashscore', 'Shots on target')
                    ('shots_on_target', 'sofascore',  'shotsOnGoal')
```

Statistik yang belum ada di kamus disimpan mentah dan di-log, tidak dibuang diam-diam.

### 7.3 Routing dan fallback

Contoh isi `provider_routes`:

| competition | data_type | urutan provider |
|---|---|---|
| `id-super-league` | score_status | flashscore → sofascore → livescore |
| `id-super-league` | events | flashscore → sofascore |
| `id-super-league` | statistics | sofascore → flashscore |
| `id-super-league` | lineups | flashscore → sofascore |
| `id-super-league` | player_profile | sofascore → transfermarkt |
| `id-liga-nusantara` | * | sofascore |
| `*` (default) | score_status | flashscore → sofascore |

Resolusi route: `(kompetisi, jenis data)` → `(kompetisi, *)` → `(*, jenis data)`.

Kapan pindah ke provider berikutnya:

| Kondisi | Aksi |
|---|---|
| Error jaringan, 5xx, timeout | Retry dengan backoff, lalu pindah |
| 403 atau 429 | Langsung pindah, buka circuit breaker provider itu |
| Sukses tapi data **kosong** | Pindah. Ini bukan error |
| Belum ada `external_ref` | Lewati provider itu, catat metrik `missing_ref` untuk ditindaklanjuti di discovery |
| Semua provider gagal | Pakai data terakhir (tandai `stale`), kirim alert |

Sketsa router:

```go
func (r *Router) Fetch(ctx context.Context, compID string, dt DataType, entityID uuid.UUID) (Block, error) {
    for _, p := range r.routes.Resolve(compID, dt) {
        if r.breaker.Open(p) { continue }
        ref, ok := r.refs.Lookup(p, entityID)
        if !ok { r.metrics.MissingRef(p, dt); continue }
        b, err := r.fetchers[p].Fetch(ctx, dt, ref)
        if err == nil && !b.Empty() { return b.WithSource(p), nil }
        r.metrics.Fallback(p, dt, err)
    }
    return Block{}, ErrAllProvidersFailed
}
```

Router menjamin satu blok (misalnya seluruh event satu pertandingan) berasal dari satu provider, sesuai aturan 6.6.

### 7.4 Similarity yang aman

Nama saja tidak cukup: "Liga 1" bisa berarti Liga 1 Indonesia, Liga 1 Rumania, atau Liga 1 Putri.

1. **Filter keras (wajib sama):** negara, gender, kelompok umur, tipe liga/piala. Kandidat yang tidak lolos dibuang.
2. **Normalisasi:** huruf kecil, hapus aksen dan tanda baca, hapus kata umum ("FC", "PS", "Persatuan", "Club", "Sepak Bola"), terapkan kamus sinonim.
3. **Skor gabungan:** trigram (`pg_trgm`) dan Jaro-Winkler, ditambah bonus atribut pendukung:
   - Kompetisi: jumlah tim sama, tanggal musim beririsan
   - Tim: pernah main di jadwal yang sama
   - Pemain: tanggal lahir sama (penentu terkuat; nama pemain Indonesia sering disingkat atau satu kata)
4. **Ambang awal** (dikalibrasi setelah ada data):
   - ≥ 0,92 dan hanya satu kandidat → auto-link
   - 0,75–0,92 atau lebih dari satu kandidat → review queue
   - < 0,75 → abaikan
5. **Setiap link menyimpan `method` dan `confidence`.** Link bisa dibatalkan, dan link `manual` tidak pernah ditimpa link otomatis.

### 7.5 Catatan

- Salah link di level kompetisi merusak semua data di bawahnya. Karena itu kompetisi prioritas di-link manual; similarity paling berguna untuk tim dan pemain yang jumlahnya ribuan.
- Review queue butuh antarmuka (minimal endpoint admin). Tanpa itu antrean akan menumpuk.

## 8. Hasil spike SofaScore (28 September 2026)

Skrip: `scrap-claude/sofascore_spike.py` (mode `--ssr` dan `--sitemap`). Diuji dari IP Indonesia. JSON mentah di `scrap-claude/sofascore_spike/` (di-ignore git).

### 8.1 Akses API: terblokir

| HTTP client | `/api/v1/...` | Isi respons |
|---|---|---|
| urllib biasa | 403 | `"reason": "Forbidden"` (ditolak di TLS/header) |
| curl_cffi (TLS Chrome) | 403 | `"reason": "challenge"` (lolos TLS, butuh token challenge) |
| Playwright (Chromium headless, `fetch` dari dalam halaman) | 403 | `"reason": "challenge"` |

Sama untuk `www.sofascore.com/api/v1` dan `api.sofascore.com/api/v1`. Halaman HTML-nya sendiri 200.

### 8.2 Yang berhasil: data di HTML (`__NEXT_DATA__`) dan sitemap

Situs dibangun dengan Next.js; setiap halaman membawa datanya di `<script id="__NEXT_DATA__">`. Cukup curl_cffi, tanpa browser. Jangan kirim `Accept: application/json` untuk halaman HTML.

| Halaman | Data yang tersedia | Tidak tersedia |
|---|---|---|
| Kompetisi `/football/tournament/<negara>/<slug>/<ut_id>` | `uniqueTournament` (ID, kategori negara), semua musim (Super League: 15 musim, terbaru 26/27 `season_id=100394`), **klasemen**, info musim (gol, kartu) | Daftar pertandingan (hanya flag `hasEvents`) |
| Tim `/football/team/<slug>/<id>` | Profil tim, **skuad lengkap** (Super League: 598 pemain, 596 dengan tanggal lahir), kompetisi yang diikuti, transfer | Jadwal/hasil tim |
| Pertandingan `/football/match/<slug>/<customId>` | Tim, turnamen, musim, babak, venue, status, **skor**, kick-off | Event (gol, kartu), statistik, lineup (hanya flag `initialHasLineups`) |
| Sitemap turnamen (`en_sitemap_tournaments_football.xml.gz`) | 5.262 turnamen sepak bola dunia | |
| Sitemap pertandingan (45 file, ±437 rb URL, ±63 MB) | URL pertandingan (slug tim + `customId`), termasuk musim 26/27 dan jadwal yang belum dimainkan | Kompetisi dan tanggal (harus buka halaman pertandingan). **Tidak lengkap** (lihat 8.4) |

`robots.txt`: `Disallow: /` hanya untuk `Bytespider`; untuk `*` hanya beberapa path (antara lain `/standings/`, tanggal lama). Risiko ToS tetap berlaku (bagian 4).

### 8.3 Turnamen Indonesia di SofaScore (dari sitemap)

| ut_id | Slug | Keterangan |
|---|---|---|
| 1015 | `liga-1` | Indonesia Super League |
| 23229 | `indonesia-liga-2` | Championship |
| 24432 | `indonesia-liga-3` | Liga 3 / Nusantara |
| 23013 | `indonesia-presidents-cup` | Piala Presiden |
| 30693 | `epa-super-league-u20` | EPA U-20 |
| 24801 | `liga-topskor` | Liga Topskor |
| 24823, 24832, 24833, 24834 | `liga-topskor-u13` … `u16` | Liga Topskor kelompok umur |

Tidak ditemukan di sitemap: Piala Indonesia, Liga 4, Liga 1 Putri. Kompetisi AFC/ASEAN yang diikuti klub Indonesia ada di kategori Asia (misalnya ACL Two `668`, AFC Challenge League `22795`, ASEAN Club Championship `28308`).

### 8.4 Keterbatasan

- **Daftar pertandingan tidak lengkap.** Pemindaian semua sitemap hanya menemukan 91 pertandingan antar-tim Super League (satu musim penuh = 306), tersebar di beberapa musim. Tanpa API, tidak ada cara lengkap untuk mendapatkan semua ID pertandingan satu musim.
- **Tidak ada detail pertandingan** (event, statistik, lineup) di HTML.
- **Pencocokan URL pertandingan ke kompetisi** harus lewat slug tim, lalu membuka halaman pertandingan (1 request per pertandingan).

### 8.5 Kesimpulan dan keputusan yang dibutuhkan

SofaScore (tanpa API) **layak sebagai sumber katalog**: turnamen, musim, klasemen, tim, dan pemain dengan tanggal lahir. Ini cocok untuk master data dan matching (bagian 7).

SofaScore (tanpa API) **belum layak sebagai sumber skor dan jadwal** untuk kompetisi Indonesia selain Liga 1 dan Championship, yang merupakan tujuan awal Fase 3. Pilihan:

1. **Flashscore untuk kompetisi tersebut**, bila tersedia di sana. Verifikasi slug Flashscore untuk Liga 3/Nusantara, Piala Presiden, EPA, Liga Topskor.
2. **API-Football (resmi, berbayar)** untuk kompetisi yang tidak ada di Flashscore.
3. **Riset lanjutan challenge SofaScore** (reverse-engineer token): tidak disarankan. Rapuh, dan jelas berupa upaya melewati proteksi.

Rekomendasi: katalog dari SofaScore (HTML), skor dan detail dari Flashscore untuk semua kompetisi Indonesia yang tersedia di sana, API-Football sebagai cadangan.
