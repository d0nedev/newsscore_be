# Deploy ke server lewat SSH

Langkah dari server kosong sampai API jalan di `https://$DOMAIN`, memakai stack di [`deploy/`](../deploy/): Caddy (HTTPS otomatis), app, ingestor, Postgres, migrate, dan OTel Collector, semuanya Docker Compose di satu server. Alert, dashboard Grafana Cloud, dan penanganan insiden ada di [`runbook.md`](runbook.md#deploy-vps-deploy).

Contoh di bawah memakai Ubuntu 24.04, user `deploy`, dan alias SSH `newsscore`. Perintah bertanda **lokal** dijalankan di laptop, sisanya di server.

## Kebutuhan

| Apa | Catatan |
|---|---|
| Server | Min. 2 vCPU, 4 GB RAM (Postgres dibatasi 3 GB, app 512 MB, ingestor 256 MB), 20 GB disk |
| Domain | `A` record mengarah ke IP server **sebelum** start pertama, supaya Caddy dapat sertifikat Let's Encrypt |
| Grafana Cloud | Stack gratis cukup. **Connections > OpenTelemetry (OTLP)**: endpoint, instance ID, token. Wajib: compose menolak start tanpa tiga nilai ini |
| Akses repo | Repo privat: deploy key read-only di server |

## 1. Siapkan server (sekali)

Login pertama sebagai root:

```sh
adduser --disabled-password --gecos "" deploy
usermod -aG sudo deploy
mkdir -p /home/deploy/.ssh
cp ~/.ssh/authorized_keys /home/deploy/.ssh/
chown -R deploy:deploy /home/deploy/.ssh && chmod 700 /home/deploy/.ssh
passwd deploy   # password untuk sudo; login SSH tetap pakai key
```

**Lokal**, tambahkan ke `~/.ssh/config`, lalu pastikan `ssh newsscore` masuk tanpa password **sebelum** lanjut:

```
Host newsscore
    HostName <ip-server>
    User deploy
    IdentityFile ~/.ssh/id_ed25519
```

Di server sebagai `deploy`: matikan login password dan root, nyalakan firewall dan security update otomatis.

```sh
sudo tee /etc/ssh/sshd_config.d/10-hardening.conf >/dev/null <<'EOF'
PasswordAuthentication no
PermitRootLogin no
EOF
sudo systemctl reload ssh

sudo ufw allow OpenSSH && sudo ufw allow 80/tcp && sudo ufw allow 443 && sudo ufw --force enable
sudo apt update && sudo apt install -y unattended-upgrades git && sudo dpkg-reconfigure -plow unattended-upgrades
```

Docker Engine + compose plugin:

```sh
curl -fsSL https://get.docker.com | sudo sh
sudo usermod -aG docker deploy
exit   # login ulang supaya grup docker berlaku
```

> Port yang di-publish Docker melewati `ufw`. Stack ini hanya mem-publish 80/443 (Caddy); Postgres dan collector tidak pernah terbuka ke luar. Jangan menambah `ports:` lain di `deploy/docker-compose.yml`.

## 2. Ambil kode

Repo privat: buat deploy key, daftarkan di GitHub (**Settings > Deploy keys**, read-only).

```sh
ssh-keygen -t ed25519 -N "" -f ~/.ssh/github_deploy
cat ~/.ssh/github_deploy.pub   # tempel ke GitHub
cat >> ~/.ssh/config <<'EOF'
Host github.com
    IdentityFile ~/.ssh/github_deploy
EOF

git clone git@github.com:d0nedev/newsscore_be.git ~/newsscore
cd ~/newsscore/deploy
```

## 3. Isi `.env`

```sh
cp .env.example .env && chmod 600 .env
openssl rand -hex 32   # untuk POSTGRES_PASSWORD; harus URL-safe karena dipakai di URL migrate
nano .env
```

Wajib diisi: `DOMAIN`, `POSTGRES_PASSWORD`, `GRAFANA_CLOUD_*`, `CORS_ALLOWED_ORIGINS` (origin frontend persis, mis. `https://newsscore.id`). Opsional: `INGEST_INTERVAL`, `INGEST_DETAILS_PER_RUN`, `INGEST_LIVE_INTERVAL`, `FLASHSCORE_FSIGN`.

## 4. Start pertama

Ada dua pilihan. Pilih salah satu.

### A. Database kosong

Ingestor mengisi semuanya dari nol. Halaman liga terisi dalam beberapa menit, detail laga dan skuad menyusul berjam-jam lewat antrean (lihat [`ingestor.md`](ingestor.md)). Liga SofaScore butuh `seed-sofascore`, yang biasanya diblokir captcha dari IP server; untuk data itu pakai pilihan B.

```sh
VERSION=$(git describe --tags --always) docker compose up -d --build
```

### B. Bawa data dari lokal

Lebih cepat, dan membawa data SofaScore yang sudah di-seed. Server harus berada di commit yang sama dengan lokal, karena dump membawa versi migrasinya.

**Lokal**, dump database dan kirim:

```sh
pg_dump -h localhost -U newsscore_admin -d newsscore -Fc --no-owner --no-acl -f newsscore.dump
scp newsscore.dump newsscore:~/newsscore/deploy/
```

Di server, nyalakan Postgres saja, restore, baru start semuanya. `migrate` melihat versi dari dump dan tidak mengubah apa pun.

```sh
cd ~/newsscore/deploy
docker compose up -d postgres
docker compose exec -T postgres sh -c 'pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --no-owner --exit-on-error' < newsscore.dump
VERSION=$(git describe --tags --always) docker compose up -d --build
rm newsscore.dump
```

**Lokal**, salin logo dan foto (~300 MB) ke volume `assets`. Pemiliknya harus UID 65532 (user `nonroot` image app):

```sh
tar -C data/assets -czf - . | ssh newsscore \
  'docker run --rm -i -v newsscore-prod_assets:/a alpine sh -c "tar -xzf - -C /a && chown -R 65532:65532 /a"'
```

## 5. Cek

```sh
docker compose ps                          # migrate: exited (0), lainnya running
curl -fsS https://$DOMAIN/ready            # 200
curl -fsS "https://$DOMAIN/api/v1/leagues" | head -c 300
docker compose logs -f --tail=50 ingestor  # cari "job progress", tanpa "source blocked us"
```

## 6. Akun admin

`useradd` tidak ada di image app, jadi jalankan dari source dengan container Go di network compose yang sama:

```sh
cd ~/newsscore
set -a; . deploy/.env; set +a
printf '%s\n' 'password-kuat' | docker run --rm -i --network newsscore-prod_default \
  -v "$PWD":/src -w /src \
  -e APP_ENV=production -e DB_HOST=postgres -e DB_SSL_MODE=require \
  -e DB_USER="$POSTGRES_USER" -e DB_PASSWORD="$POSTGRES_PASSWORD" -e DB_NAME="$POSTGRES_DB" \
  golang:1.27.4 go run ./cmd/useradd -email admin@example.com -name Admin -admin
```

## 7. Backup harian

```sh
crontab -e
# 15 2 * * * /home/deploy/newsscore/deploy/backup.sh >> /home/deploy/newsscore-backup.log 2>&1
```

Dump disimpan 7 hari di `deploy/backups/`. Set `BACKUP_REMOTE` (remote rclone, mis. `r2:newsscore-backups`) supaya ada salinan di luar server; backup di disk yang sama ikut hilang bersama servernya.

## Rilis versi baru

**Lokal**: push ke `main`. Lalu:

```sh
ssh newsscore 'cd ~/newsscore && git pull --ff-only && cd deploy && VERSION=$(git describe --tags --always) docker compose up -d --build && docker image prune -f'
```

- Migrasi baru jalan otomatis sebelum app start, jadi harus kompatibel dengan versi app sebelumnya.
- Satu replika: Caddy menjawab 502 beberapa detik selama container app diganti.
- Ingestor ikut restart; job yang sedang `running` diambil lagi setelah 15 menit.

**Rollback**: `git checkout <tag-atau-commit-sebelumnya>` lalu jalankan perintah build yang sama. Migrasi tidak ikut mundur.

## Perintah sehari-hari

```sh
cd ~/newsscore/deploy
docker compose logs -f app                 # log lokal, 5 x 10 MB per container
docker compose restart ingestor
docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB"'
```

Akses Postgres dari laptop tanpa membuka port: tunnel SSH ke IP container.

```sh
ssh newsscore "docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' newsscore-prod-postgres-1"
ssh -N -L 15432:<ip-container>:5432 newsscore   # lalu psql -h localhost -p 15432 ...
```

## SofaScore proxy (opsional)

Dari IP server, API SofaScore biasanya minta captcha (403 `challenge`), jadi skor Liga Nusantara, EPA, dan Liga Topskor tidak diperbarui. `scrap-claude/sofascore_proxy.py` menjalankan Firefox (Camoufox) asli yang lolos captcha, lalu ingestor memanggil API lewat browser itu. Butuh RAM sekitar 500 MB sampai 1 GB tambahan.

```sh
sudo apt install -y xvfb python3-venv
python3 -m venv ~/sofaproxy && ~/sofaproxy/bin/pip install "camoufox[geoip]" && ~/sofaproxy/bin/python -m camoufox fetch

# Container -> host:8787. Hanya dari jaringan Docker; port tetap tertutup dari internet.
sudo ufw allow from 172.16.0.0/12 to any port 8787 proto tcp

sudo tee /etc/systemd/system/sofaproxy.service >/dev/null <<'EOF'
[Unit]
Description=SofaScore captcha proxy
After=network-online.target

[Service]
User=deploy
ExecStart=/home/deploy/sofaproxy/bin/python /home/deploy/newsscore/scrap-claude/sofascore_proxy.py --host 0.0.0.0 --virtual
Restart=always
RestartSec=30

[Install]
WantedBy=multi-user.target
EOF
sudo systemctl daemon-reload && sudo systemctl enable --now sofaproxy
journalctl -u sofaproxy -f        # tunggu "token ok"
```

Lalu di `deploy/.env` set `SOFASCORE_PROXY=http://host.docker.internal:8787` dan `docker compose up -d ingestor`. Cek:

```sh
curl -s localhost:8787/api/v1/unique-tournament/17/seasons | head -c 200    # JSON, bukan 403
docker compose logs --since=2h ingestor | grep -i sofascore                  # tanpa "sync sofascore failed"
```

Proxy tidak punya autentikasi: jangan buka port 8787 ke internet. Kalau log proxy terus "no token after 60s", IP server ditandai keras oleh Cloudflare; isi data SofaScore dari lokal (start pertama pilihan B).

## Batasan

- SofaScore tanpa proxy di atas: sync berhenti di 403 tiap jam. Tidak berbahaya, tapi skor liga SofaScore tidak diperbarui.
- Satu server adalah single point of failure. Pasang uptime check Grafana Cloud ke `https://$DOMAIN/ready` (lihat [`runbook.md`](runbook.md#deploy-vps-deploy)).
