# Seed data SofaScore ke prod dari lokal

Tanggal: 2026-11-11. Referensi: [`p1-sofascore-proxy-prod.md`](p1-sofascore-proxy-prod.md), [`docs/deployment.md`](../deployment.md#refresh-data-sofascore-dari-lokal-coolify).

## Item

| ID | Masalah | Perbaikan | File |
|---|---|---|---|
| R1 | Prod tanpa kompetisi SofaScore (0 dari 28), IP server diblokir SofaScore | DB prod diganti dump lokal (35 kompetisi, 7 SofaScore, 1909 laga SofaScore) | server |
| R2 | Volume assets prod 4,9 MB, tanpa foto pemain | Salin `data/assets` (309 MB), `chown 65532` | server |
| R3 | Prosedur belum tertulis | Bagian "Refresh data SofaScore dari lokal (Coolify)" | `docs/deployment.md` |

Kenapa ganti total, bukan merge: UUID prod berbeda dengan lokal (mis. Indonesia `7889b9dc...` vs `5cf66c2e...`), dan tabel prod yang tidak ada di lokal (`users`, `sessions`, `news`, `news_links`) kosong. Kedua DB di migrasi 30.

## Test

Tidak ada test baru (operasi data).

## Verifikasi

- Backup prod sebelum restore: `/root/backups/newsscore-prod-before-local-restore-20261111.dump` (16 tabel data).
- Setelah restore: `competitions` 35, `matches.sofascore_id` 1909, `schema_migrations` 30 tidak dirty.
- `https://api.newsscore.id/ready`: 200. `/api/v1/leagues` memuat `liga-topskor*`. Asset `/assets/leagues/president-cup.png`: 200.
- Ingestor jalan: `competition scraped`, `team page synced`, `job progress`.

## Temuan

- `sync images failed: permission denied` selama sekitar 3 menit, saat `tar` belum sampai ke `chown`. Hilang setelah ingestor restart.
- `sync sofascore failed: blocked by sofascore: status 403` tiap jam: memang begitu, data SofaScore di prod statis.
- `scrape squad failed ... /team/nusantara/vgjR4Ao3/squad/ 404`: tim Flashscore tanpa halaman skuad, sudah ada sebelumnya.
- Dari ISP lokal, `www.sofascore.com` di-resolve ke `lamanlabuh.aduankonten.id` (Internet Positif). Seed ulang dari laptop butuh VPN.

## Terbuka

- Dump lokal masih ada di `/root/backups/local-20261111.dump` (server) dan `~/.jcode/scratch/local.dump`. Hapus manual bila tidak perlu.
- Paket proxy di `/opt/sofaproxy` (service mati) bisa di-uninstall.
- Data SofaScore perlu di-refresh manual berkala.
