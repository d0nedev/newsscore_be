# Runbook: newsscore

Setiap alert di `prometheus-alerts.yml` punya `annotations.runbook` yang mengarah ke bagian di dokumen ini.

## SLO

| SLO | Target | Pengukuran | Error budget (30 hari) |
|---|---|---|---|
| Availability | 99.9% request tanpa 5xx | `service:http_errors_5xx:rate5m / service:http_requests:rate5m` | ~43 menit setara 100% error |
| Latency | p95 < 300ms per route | `service:http_latency_p95:5m` | — |

Baseline load test belum ada; buat ulang setelah endpoint pertandingan tersedia.

## Sinyal Umum

| Kebutuhan | Tempat |
|---|---|
| Metrics | Prometheus. Label `http_route`, `http_response_status_code`, `service_version`, `deployment_environment_name` |
| Trace | Jaeger, service `newsscore`. Nama span mengikuti route, mis. `GET /api/v1/matches/{id}` |
| Log | JSON stdout. Korelasi lewat `request_id` (juga di header `X-Request-ID`) dan `trace_id` |
| Konfigurasi efektif | Log `configuration loaded` saat startup |

## ServiceHighErrorRate

Rasio 5xx di atas 1.4% selama 5 menit, artinya error budget bulanan habis sekitar 14× lebih cepat dari seharusnya.

1. Cari route mana yang gagal: `sum by (http_route, http_response_status_code) (rate(http_server_request_duration_seconds_count{http_response_status_code=~"5.."}[5m]))`.
2. Cek apakah bertepatan dengan rilis: pisahkan per `service_version`. Jika error hanya ada di versi baru, rollback.
3. Cari log `"level":"ERROR"` dan periksa `error_code`:
   - `<DOMAIN>_*_FAILED`: error dari database. Cek `/ready`, koneksi Postgres, dan apakah query dibatalkan oleh `statement_timeout` (`canceling statement due to statement timeout`) atau oleh batas waktu request (`context deadline exceeded`, lihat ServiceDatabasePoolSaturated).
   - `panic recovered`: bug. Lihat `stack_trace`, lalu rollback.
4. Buka trace contoh lewat `trace_id` dari log untuk melihat span DB yang gagal.

## ServiceHighLatency

p95 sebuah route di atas 300ms selama 10 menit.

1. Di Jaeger, urutkan trace route tersebut berdasarkan durasi. Bandingkan span `pool.acquire` dengan span query.
   - `pool.acquire` lama: pool DB penuh. Pertimbangkan menaikkan `DB_MAX_CONNS` (perhatikan `max_connections` Postgres dikali jumlah replika) atau menambah replika.
   - Query lama: cek `pg_stat_statements` dan rencana eksekusi. Untuk list, pastikan index yang sesuai dipakai (mis. `matches (match_time)`).
2. Pastikan tidak ada klien yang meminta `limit=100` secara berlebihan.

## ServiceDatabasePoolSaturated

Minimal satu replika memakai ≥ 90% koneksi pool (`DB_MAX_CONNS`) selama 5 menit. Request berikutnya menunggu koneksi, dan kalau menunggu melewati batas waktu request (`APP_WRITE_TIMEOUT` dikurangi margin), request gagal dengan `<DOMAIN>_*_FAILED` dan `cause` berisi `context deadline exceeded`.

1. Cek apakah query melambat: `rate(pgxpool_empty_acquire_wait_time_nanoseconds_total[5m])` naik bersamaan dengan latency, dan trace menunjukkan span query yang lama. Kalau ya, masalahnya di DB, bukan ukuran pool (lihat ServiceHighLatency).
2. Kalau query normal tapi trafik naik: tambah replika, atau naikkan `DB_MAX_CONNS`. Pastikan `DB_MAX_CONNS × jumlah replika` masih di bawah `max_connections` Postgres.
3. Kalau hanya satu `exported_instance` yang penuh: cek replika itu (koneksi bocor, request lambat tertentu).

## ServiceNoTraffic

Tidak ada metric request selama 15 menit.

1. Cek app hidup: `/health`, status pod/container, restart loop.
2. Jika app hidup dan menerima request (lihat access log), berarti masalahnya di pipeline telemetry: cek log collector dan `OTEL_EXPORTER_OTLP_ENDPOINT`.
3. Di environment dengan trafik rendah (malam/staging), alert ini bisa normal. Sesuaikan atau matikan per environment.

## ServiceTelemetryPipelineDown

Prometheus tidak bisa scrape OTel collector. **Semua alert lain buta** selama kondisi ini.

1. Cek container/pod collector dan log-nya.
2. Validasi konfigurasi: `otelcol validate --config=otel-collector.yaml`.

## Operasi Rutin

### Deploy (VPS, `deploy/`)

Topologi: satu VPS menjalankan Caddy (HTTPS otomatis), app, Postgres, migrate, dan OTel Collector. Metrics, traces, dan log dikirim ke **Grafana Cloud**, jadi alert tetap jalan saat VPS mati. Stack Jaeger/Prometheus/Grafana di `docker-compose.yml` root hanya untuk dev.

**Setup pertama:**

1. VPS: pasang Docker Engine + compose plugin, aktifkan security update otomatis (`unattended-upgrades`), SSH hanya pakai key. Firewall hanya buka 22, 80, 443.
2. DNS `A` record domain ke IP VPS (wajib sebelum start, untuk sertifikat Let's Encrypt).
3. Grafana Cloud: buat stack, lalu **Connections > OpenTelemetry (OTLP)** untuk mendapatkan endpoint, instance ID, dan token.
4. Di VPS:
   ```sh
   git clone <repo> && cd <repo>/deploy
   cp .env.example .env && chmod 600 .env   # isi semua nilai
   VERSION=$(git describe --tags --always) docker compose up -d --build
   curl -fsS https://$DOMAIN/ready
   ```
5. Cron backup (lihat header `deploy/backup.sh`) dan set `BACKUP_REMOTE` ke object storage di luar VPS.
6. Grafana Cloud:
   - **Alert rules**: import `prometheus-alerts.yml`. Metrics lewat OTLP memakai label `job`/`instance`, bukan `exported_job`/`exported_instance`:
     `sed 's/exported_job/job/g; s/exported_instance/instance/g' prometheus-alerts.yml`.
     Lewati `ServiceTelemetryPipelineDown` (tidak ada scrape Prometheus); penggantinya `ServiceNoTraffic` + uptime check.
   - **Dashboard**: import `grafana/dashboards/newsscore.json` dengan substitusi label yang sama.
   - **Uptime check** (Synthetic Monitoring) ke `https://$DOMAIN/ready` dari luar. Ini satu-satunya alert yang menangkap VPS mati total.
   - **Contact point**: arahkan notifikasi ke Slack/email/pager.
   - **Log**: Explore > Loki, `{service_name="newsscore"}`. Klik `trace_id` untuk membuka trace di Tempo.

**Rilis versi baru:**

```sh
cd <repo> && git pull && cd deploy
VERSION=$(git describe --tags --always) docker compose up -d --build
docker image prune -f
```

Migrasi jalan otomatis (service `migrate`) sebelum app start, jadi migrasi harus kompatibel dengan versi app sebelumnya. Dengan satu replika, ada jeda beberapa detik (Caddy mengembalikan 502) selama container app diganti.

**Rollback:** `git checkout <tag-sebelumnya>` lalu jalankan perintah rilis yang sama. Migrasi tidak ikut mundur; turunkan manual hanya bila perlu.

**Restore backup:**

```sh
docker compose exec -T postgres sh -c 'pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --clean --if-exists' < backups/<file>.dump
```

Uji restore ke database terpisah secara berkala; backup yang tidak pernah di-restore belum terbukti.

**Log lokal di VPS:** `docker compose logs -f app`. Tiap container dibatasi 5 × 10 MB. Log collector tidak dikirim ke Grafana Cloud (mencegah loop saat export gagal); cek dengan `docker compose logs otel-collector`.

### Shutdown / Rolling Restart

Saat SIGTERM: `/ready` langsung 503, lalu tunggu `APP_SHUTDOWN_DRAIN_DELAY`, lalu request yang sedang berjalan diselesaikan dalam `APP_SHUTDOWN_TIMEOUT`, lalu telemetry di-flush. Pastikan `drain delay + shutdown timeout` < grace period orchestrator (Kubernetes default 30s). Exit code 0 berarti berhenti bersih; exit code 1 berarti server gagal atau shutdown HTTP melebihi timeout.

### Rotasi API Key

1. Tambahkan key baru: `API_KEYS=old,new`, lalu deploy.
2. Pindahkan semua klien ke key baru.
3. Hapus key lama: `API_KEYS=new`, lalu deploy.

### Batasan yang Diketahui

- Rate limit disimpan di memori per instance. Dengan N replika, limit efektif adalah N × `RATE_LIMIT_REQUESTS_PER_MINUTE`.
- Tidak ada Alertmanager di `docker-compose.yml` (dev). Di production, alert dan routing notifikasi ada di Grafana Cloud.
- Deploy VPS satu replika: tidak zero-downtime saat rilis, dan VPS adalah single point of failure. Backup off-site adalah jaring pengaman utamanya.
