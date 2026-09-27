# 02 — Arsitektur Sistem

## 1. Gambaran umum

```
                    ┌──────────────────────────────┐
                    │   Flashscore (feed publik)   │
                    └──────────────┬───────────────┘
                                   │ HTTP scrape, terjadwal
                                   ▼
┌─────────────────────────────────────────────────────────────┐
│  ingestor (Go)                                              │
│  scheduler  →  anti-corruption layer  →  differ  →  events  │
└───────────┬──────────────────────────────────┬──────────────┘
            │ tulis                            │ publish
            ▼                                  ▼
     ┌─────────────┐                    ┌─────────────┐
     │ PostgreSQL  │◄───── baca ───────►│    Redis    │
     │  (sumber    │                    │ cache +     │
     │  kebenaran) │                    │ pub/sub     │
     └─────────────┘                    └──────┬──────┘
            ▲                                  │ subscribe
            │ baca                             ▼
┌───────────┴──────────────────────────────────────────────┐
│  api (Go)                                                │
│  REST  ·  SSE hub  ·  admin CMS                          │
└───────────────────────────┬──────────────────────────────┘
                            │ HTTP + SSE
                            ▼
                     Klien (frontend terpisah)
```

Dua binary Go terpisah, satu basis data, satu Redis. Frontend di luar lingkup
dokumen ini.

## 2. Keputusan arsitektur

### 2.1 Ingestor terpisah dari API

**Keputusan.** `cmd/ingestor` dan `cmd/api` adalah dua proses berbeda.

**Alasan.** Keduanya punya karakter beban yang bertolak belakang. Ingestor
berjalan konstan dengan beban yang bisa diramalkan; API melonjak tajam saat
pertandingan besar. Kalau digabung, lonjakan trafik menunda ingestion dan skor
justru tertinggal tepat ketika paling banyak dilihat. Pemisahan juga membuat
API bisa direstart tanpa memutus siklus ingestion.

**Konsekuensi.** Perlu koordinasi antarproses lewat PostgreSQL `LISTEN/NOTIFY`, dan deployment
mengelola dua unit. Keduanya dapat diterima.

### 2.2 PostgreSQL sebagai sumber kebenaran, bukan cache belaka

**Keputusan.** Seluruh data dari Flashscore disimpan permanen di PostgreSQL.

**Alasan.** Tiga hal menuntutnya. Berita perlu ditautkan ke pertandingan dan
klub, sehingga entitas itu harus ada sebagai baris yang bisa direferensikan.
Halaman harus tetap tersaji ketika Flashscore memblokir atau mengubah format. Dan data historis
tidak perlu di-scrape ulang, yang justru memperbesar risiko diblokir.

**Konsekuensi.** Kebutuhan penyimpanan bertambah dan ada pekerjaan migrasi
skema. Sebagai imbalan, ketergantungan operasional pada Flashscore berkurang
drastis.

### 2.3 Anti-corruption layer terhadap penyedia

**Keputusan.** Format feed Flashscore (`¬~`, `¬`, `÷`, kode key seperti `AA`,
`AE`) tidak pernah menembus keluar dari paket `internal/provider/flashscore`. Di batas paket, ia diterjemahkan ke
model domain milik sendiri.

**Alasan.** Feed scraping tidak punya kontrak: key, header `x-fsign`, dan
regex ekstraksi bisa berubah tanpa pemberitahuan, dan penyedia mungkin harus
diganti seluruhnya (`internal/provider/sofascore` sudah ada sebagai cadangan). Kalau bentuk mereka bocor ke seluruh basis kode,
setiap perubahan menjadi pekerjaan besar dan penggantian penyedia menjadi
mustahil.

**Konsekuensi.** Ada kode pemetaan yang terasa berulang di awal. Ini harga
yang murah dibanding alternatifnya.

### 2.4 Server-Sent Events, bukan WebSocket atau polling

**Keputusan.** Pembaruan langsung dikirim lewat SSE pada satu endpoint.

**Alasan.** Alur datanya satu arah — server memberi tahu klien, klien tidak
pernah mengirim balik. WebSocket menambah kompleksitas dua arah yang tidak
terpakai. SSE berjalan di atas HTTP biasa, menembus proxy tanpa penyesuaian,
dan otomatis menyambung ulang tanpa kode tambahan. Polling dikesampingkan
karena dengan seribu tab terbuka pada interval 30 detik, biaya permintaannya
berlipat tanpa memberi kesegaran yang lebih baik.

**Konsekuensi.** Batas enam koneksi per domain pada HTTP/1.1 menjadi masalah,
sehingga HTTP/2 wajib aktif di reverse proxy.

### 2.5 sqlc, bukan ORM

**Keputusan.** Kueri ditulis sebagai SQL, lalu sqlc menghasilkan kode Go
bertipe.

**Alasan.** Beban kerjanya didominasi baca dengan join yang rumit dan agregasi
untuk klasemen serta statistik. ORM menyembunyikan SQL yang dihasilkan tepat
pada titik di mana SQL itu paling perlu dikendalikan. sqlc memberi tipe yang
aman tanpa menyembunyikan apa pun, dan kesalahan kueri terdeteksi saat kompilasi.

## 3. Alur data

### 3.1 Pertandingan tidak berlangsung

```
Klien → API → Redis (hit) → JSON
```

Sebagian besar permintaan berhenti di Redis. PostgreSQL hanya tersentuh saat
cache meleset.

### 3.2 Pertandingan berlangsung

```
Ingestor ──20 detik──► Flashscore
    │
    ├─► deteksi perubahan ─► simpan ke PostgreSQL
    │
    └─► trigger pg_notify ─► API (LISTEN) ─► SSE hub ─► seluruh klien terhubung
```

Klien menerima muatan awal lewat REST, lalu hanya menerima selisihnya lewat SSE.
Halaman tidak pernah memuat ulang dirinya.

### 3.3 Ketika Flashscore terganggu atau memblokir

Tiga lapis penurunan bertingkat:

1. Cache masih segar — sajikan seperti biasa.
2. Cache kedaluwarsa — sajikan data lama disertai `data_as_of`, dan antarmuka
   menampilkan waktu pembaruan terakhir.
3. Cache kosong — baca dari PostgreSQL, yang selalu punya kondisi terakhir yang
   diketahui.

Kondisi cache kosong total praktis tidak terjadi, karena PostgreSQL menyimpan
semuanya. Inilah alasan keputusan 2.2 penting.

## 4. Strategi cache

| Data | TTL Redis | Catatan |
|---|---|---|
| Pertandingan berlangsung | 15 detik | Dibatalkan lebih awal oleh event dari ingestor |
| Pertandingan hari ini | 3 menit | |
| Pertandingan selesai | 6 jam | Praktis tidak berubah |
| Klasemen | 15 menit | |
| Skuad dan profil klub | 12 jam | |
| Profil pemain | 24 jam | |
| Daftar berita | 2 menit | Dibatalkan saat artikel diterbitkan |
| Artikel berita | 30 menit | Dibatalkan saat artikel disunting |

Dua pola yang wajib diterapkan:

**Stale-while-revalidate.** Permintaan selalu dijawab dari cache; penyegaran
berjalan di latar belakang. Pengguna tidak pernah menunggu Flashscore.

**Single-flight.** Ketika cache kedaluwarsa dan seratus permintaan datang
bersamaan, hanya satu yang menembus ke hulu. Sisanya menunggu hasil yang sama.
Tanpa ini, lonjakan permintaan berubah jadi lonjakan scrape dan IP server
diblokir Cloudflare. API tidak pernah memanggil Flashscore langsung; hanya
ingestor yang boleh.

## 5. Deployment

Satu VPS, Docker Compose, Caddy sebagai reverse proxy dengan TLS otomatis dan
HTTP/2 aktif.

```
caddy      → 80, 443
api        → binary Go
ingestor   → binary Go
postgres   → volume persisten
redis      → mode append-only
```

Kebutuhan minimum: 4 GB RAM, 2 vCPU. Basis data dicadangkan harian ke
penyimpanan objek eksternal, dan prosedur pemulihannya harus diuji sebelum
peluncuran — cadangan yang belum pernah dipulihkan bukan cadangan.

Jalur skala berikutnya, kalau dibutuhkan: pindahkan aset statis ke CDN,
jalankan beberapa instans API di belakang Caddy, dan pisahkan PostgreSQL ke
mesin tersendiri. Ingestor tetap satu instans; menjalankannya ganda akan
menghasilkan event berlipat.

## 6. Observability

- **Log terstruktur** dengan `log/slog` dalam format JSON, membawa `request_id`
  yang juga dikembalikan ke klien lewat header agar keluhan pengguna dapat
  ditelusuri.
- **Sentry** pada API dan ingestor.
- **Endpoint kesehatan** `/healthz` untuk liveness dan `/readyz` yang memeriksa
  PostgreSQL dan Redis.
- **Metrik yang dipantau**: rasio cache hit, tingkat galat scrape (403, respons kosong, gagal parse), jumlah
  koneksi SSE aktif, jeda ingestion, dan latensi persentil ke-95.
- **Peringatan otomatis** ke Telegram untuk: tingkat galat hulu melonjak, rasio
  cache hit turun di bawah 85 persen, ingestion berhenti lebih dari 2 menit saat
  ada pertandingan berlangsung.
