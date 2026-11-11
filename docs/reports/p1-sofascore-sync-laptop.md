# Sync SofaScore otomatis dari laptop ke prod

Tanggal: 2026-11-11. Referensi: [`p1-seed-sofascore-prod.md`](p1-seed-sofascore-prod.md), [`docs/deployment.md`](../deployment.md#sync-otomatis-dari-laptop-rekomendasi).

## Item

| ID | Masalah | Perbaikan | File |
|---|---|---|---|
| A1 | Data SofaScore prod statis (IP server diblokir) | Script laptop: SSH tunnel ke Postgres prod, `cmd/seed-sofascore` menulis langsung ke prod | `scripts/sofascore-sync.sh` |
| A2 | Harus dijalankan manual | systemd user service + timer (5 menit setelah boot, lalu 1 jam setelah run selesai) | `scripts/systemd/sofascore-sync.{service,timer}` |
| A3 | Prosedur | Bagian "Sync otomatis dari laptop" | `docs/deployment.md` |

Desain: tanpa dump/restore. Upsert per `sofascore_id` sama dengan ingestor, jadi data Flashscore, `users`, `news` di prod tidak tersentuh. Kredensial DB diambil dari container per run, tidak disimpan di laptop. Postgres tetap tidak terbuka ke internet.

## Test

Tidak ada test Go baru (script shell + unit systemd). `bash -n` OK.

## Verifikasi

- Jaringan: ZTE 5G + ProtonVPN NL. `curl` polos 403, tapi client Go provider (`tls-client`, fingerprint browser) lolos: `sofascore.LatestSeason(24432)` mengembalikan season 2026.
- Run manual pertama dihentikan `timeout 1500` (exit 124) setelah 25 menit, sebelum selesai. Sempat `sofascore competition synced` untuk epa-super-league-u20 (182 laga), indonesia-liga-3 (168), liga-topskor (251), u13 (354), u14 (379), u15 (354). Upsert, jadi terputus di tengah tidak merusak data; run systemd berikutnya melanjutkan.
- Prod: laga SofaScore 1909 menjadi 2171 di tengah run, `updated_at` terbaru 2026-11-11 14:02 UTC.
- `systemctl --user enable --now sofascore-sync.timer`: aktif, run kedua langsung jalan (tunnel + seed terlihat di cgroup). Linger user sudah `yes`.

## Temuan

- `curl` ditolak tapi client Go lolos: SofaScore memfilter fingerprint TLS, bukan hanya IP.
- Koneksi tunnel muncul di prod sebagai `127.0.0.1` lewat sshd, bukan `client_addr` laptop.

## Terbuka

- Bergantung laptop menyala dan jaringan yang lolos. Otomatis penuh butuh proxy residential.
- Path absolut di `sofascore-sync.service` (`/home/danni/...`).
- Bersih-bersih dari laporan sebelumnya (dump lama, `/opt/sofaproxy`) masih menunggu persetujuan.
