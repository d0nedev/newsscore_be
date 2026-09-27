# LigaLive — Dokumentasi Persiapan Development

Platform skor langsung dan berita olahraga, web-only, fokus pasar Indonesia.

## Objective

Membangun destinasi web tempat pendukung sepak bola Indonesia mengecek skor
dan membaca berita dalam satu tempat, dengan kedalaman liga lokal yang tidak
disediakan platform global, dan kecepatan muat yang layak di jaringan seluler
Indonesia.

Tiga sasaran yang menentukan hampir semua keputusan teknis di dokumen ini:

1. **Trafik datang dari pencarian Google berbahasa Indonesia.** Karena itu
   server-side rendering, URL berbahasa Indonesia, dan data terstruktur bukan
   fitur tambahan melainkan persyaratan arsitektur.
2. **Perangkat pengguna adalah Android kelas menengah di jaringan 4G yang tidak
   stabil.** Karena itu ada anggaran performa yang mengikat, bukan sekadar
   aspirasi.
3. **Skor harus terasa hidup tanpa membebani server.** Karena itu pembaruan
   didorong dari server lewat satu koneksi, bukan ditarik berulang oleh setiap
   tab yang terbuka.

## Peta dokumen

| Berkas | Isi |
|---|---|
| [`01-product-brief.md`](01-product-brief.md) | Sasaran produk, pengguna, pembeda dari SofaScore, lingkup MVP, dan yang sengaja tidak dikerjakan |
| [`02-architecture.md`](02-architecture.md) | Arsitektur sistem, alur data, keputusan teknis beserta alasannya |
| [`03-backend-go.md`](03-backend-go.md) | Struktur proyek Go, skema basis data, kontrak API, ingestion, caching |
| [`06-delivery-plan.md`](06-delivery-plan.md) | Fase pengerjaan, milestone, definition of done, risiko |
| [`api-contract-plan.md`](api-contract-plan.md) | Kontrak API `/api/v1` (acuan utama bila bertentangan dengan 03) |
| [`backend-handler-service.md`](backend-handler-service.md) | Aturan handler dan service |
| [`postgres-schema-plan.md`](postgres-schema-plan.md) | Skema PostgreSQL |
| [`../files/07-scraping-flashscore.md`](../files/07-scraping-flashscore.md) | Teknik scraping Flashscore |

## Ringkasan stack

| Lapisan | Pilihan |
|---|---|
| Backend | Go 1.23, chi, pgx, sqlc |
| Basis data | PostgreSQL 16 |
| Cache & pub/sub | Redis 7 |
| Sumber data | Flashscore (scraping), SofaScore sebagai cadangan |
| Realtime | Server-Sent Events |
| Deployment | Docker Compose di VPS, Caddy sebagai reverse proxy |

## Catatan perubahan dari rencana awal

Rencana sebelumnya (QUO-2026-002) menggunakan Nuxt sebagai lapisan tunggal
dengan Cloudflare Workers sebagai proxy ke SportMonks. Sumber data kini
diganti ke scraping Flashscore. Pendekatan itu memadai
untuk MVP baca-saja, tetapi tidak lagi cocok setelah ada tiga kebutuhan:
ingestion yang berjalan terus-menerus, deteksi perubahan skor sebagai sumber
event, dan CMS berita dengan basis data sendiri. Workers dirancang untuk
eksekusi pendek per permintaan, bukan proses yang hidup terus dengan state.

Go dipilih karena tiga hal itu — scheduler, diffing, dan fan-out SSE — adalah
pekerjaan yang secara alami cocok dengan goroutine dan channel, dan karena
satu binary statis jauh lebih mudah dioperasikan di VPS tunggal daripada
runtime berbasis JVM atau Node dengan kebutuhan memori serupa.

Konsekuensinya perlu disampaikan sejak awal: **ini pekerjaan yang jauh lebih
besar daripada MVP empat pekan di QUO-2026-002.** Estimasi di
[`06-delivery-plan.md`](06-delivery-plan.md) berada di kisaran 9 pekan untuk backend.
Kalau anggaran klien masih mengacu pada penawaran lama, perbedaan ini harus
dibicarakan sebelum baris kode pertama ditulis.
