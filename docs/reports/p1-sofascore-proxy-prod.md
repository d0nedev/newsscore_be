# SofaScore proxy di server prod

Tanggal: 2026-11-11. Referensi: [`docs/deployment.md`](../deployment.md#sofascore-proxy-opsional), [`docs/ingestor.md`](../ingestor.md).

## Item

| ID | Masalah | Perbaikan | File |
|---|---|---|---|
| S1 | Proxy hanya bind `127.0.0.1`, tak terjangkau container | Flag `--host` | `scrap-claude/sofascore_proxy.py` |
| S2 | `deploy/docker-compose.yml` tidak meneruskan `SOFASCORE_PROXY` | Env + `host.docker.internal` | `deploy/docker-compose.yml`, `deploy/.env.example` |
| S3 | Pasang proxy di server prod | Xvfb + Camoufox di `/opt/sofaproxy` (user `sofaproxy`), unit `sofaproxy.service` bind `172.17.0.1:8787` | server |

## Test

Tidak ada test baru (script Python dan config).

## Verifikasi

- `docker compose config` dengan `SOFASCORE_PROXY` terisi: env dan `extra_hosts` muncul.
- Proxy di server start (`listening on http://172.17.0.1:8787`), lalu `no token after 60s` dua kali, request uji 403.
- Browser Camoufox di server membuka `https://www.sofascore.com/`: halaman berisi `{"error": {"code": 403, "reason": "Forbidden" }}`, tanpa captcha.
- `curl https://www.sofascore.com/api/v1/...` dari server: 403. `img.sofascore.com`: 200.

## Temuan

- IP server (148.230.99.150) diblokir total oleh SofaScore. Proxy browser tidak membantu. Service dimatikan (`systemctl disable --now sofaproxy`), file tetap ada.
- Prod deploy lewat Coolify (`docker-compose.coolify.yml`), bukan `deploy/`. Tidak ada firewall host (tidak ada `ufw`).
- DB prod: 28 kompetisi, 0 dengan `sofascore_id`. Kompetisi SofaScore (7 baris di lokal) belum ada di prod, jadi sync SofaScore saat ini tidak punya target sama sekali.

## Terbuka

- Sumber IP lain untuk SofaScore: proxy di laptop + reverse SSH tunnel, atau proxy residential.
- Insert 7 kompetisi SofaScore ke prod dan seed datanya dari lokal.
