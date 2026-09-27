# Panduan Teknik Scraping Flashscore (Anti-Bot Bypass)

Dokumen ini menjelaskan teknik khusus (hasil riset mendalam) untuk menarik data dari Flashscore tanpa terblokir oleh perlindungan Cloudflare Turnstile maupun pemblokiran TLS/IP, serta memecahkan format teks kustom yang mereka gunakan.

## 1. Konsep Dasar & Format Teks
Flashscore **tidak** menggunakan format JSON standar untuk payload datanya. Mereka menggunakan string *flat* dengan pemisah khusus untuk menghemat *bandwidth*.

**Karakter Pemisah (Delimiter):**
- `¬~` (Tilde) : Memisahkan antar **Record** (contoh: antar pertandingan / antar event).
- `¬` (Not Sign) : Memisahkan antar **Field** dalam satu record.
- `÷` (Divide) : Memisahkan **Key** dan **Value** (Format: `KEY÷VALUE`).

**Contoh Record:**
`AA÷KO07rBLH¬AD÷1789905600¬AE÷Persijap Jepara¬AF÷Persib Bandung`
Jika di-*parse*, akan menjadi *Map/Dictionary*:
- `AA`: `KO07rBLH`
- `AD`: `1789905600`
- `AE`: `Persijap Jepara`
- `AF`: `Persib Bandung`

---

## 2. Mengambil Data Pertandingan (Tanpa Headers)
Alih-alih melakukan *fetching* API yang diproteksi tinggi, Flashscore menanam awalan data (*hydration state*) langsung di dalam HTML halaman utama. 

- **Target URL**: `https://www.flashscore.com/football/indonesia/super-league/results/`
- **Metode**: GET (Bisa menggunakan `curl` atau Golang `http.Get` biasa).
- **Target String**: Cari variabel JavaScript `cjs.initialFeeds['results'] = { data: '...' }`

**Cara Ekstrak (Regex):**
```regex
cjs\.initialFeeds\['results'\]\s*=\s*\{\s*data:\s*`(.*?)`
```
Hasil tangkapan grup 1 adalah *string* mentah berisi daftar puluhan pertandingan terakhir.

---

## 3. Mengambil Data Detail (Events, Stats, Lineups)
Untuk data detail di dalam sebuah pertandingan, Flashscore memuatnya via AJAX secara terpisah. 
Endpoint ini dijaga ketat oleh validasi *header*.

**Header Wajib (Golden Ticket):**
```http
x-fsign: SW9D1eZo
```
*(Catatan: Kunci ini bisa berubah sewaktu-waktu oleh pihak Flashscore. Jika terjadi `403 Forbidden` atau respons kosong, inspeksi ulang halaman Flashscore di Network Tab browser untuk mencari nilai `x-fsign` yang baru).*

**Endpoint Detail:**
Misal ID Pertandingan (`AA`) adalah `KO07rBLH`.
1. **Kejadian (Events/Goals/Cards)**: 
   `GET https://www.flashscore.com/x/feed/df_sui_1_KO07rBLH`
2. **Statistik (Possession, Shots)**: 
   `GET https://www.flashscore.com/x/feed/df_st_1_KO07rBLH`
3. **Susunan Pemain (Lineups/Formations)**: 
   `GET https://www.flashscore.com/x/feed/df_li_1_KO07rBLH`

---

## 4. Referensi Pemetaan Key (Key Mapping)

### Pertandingan (Results Feed)
- `AA` = ID Pertandingan (Match ID)
- `AD` = Waktu Kick-off (Unix Timestamp)
- `AE` = Nama Tim Kandang
- `AF` = Nama Tim Tandang
- `AG` = Skor Kandang
- `AH` = Skor Tandang
- `OA` = ID Gambar Logo Kandang
- `OB` = ID Gambar Logo Tandang

### Match Events (`df_sui_1`)
- `III` = Event ID
- `IA` = Sisi Tim (1 = Kandang, 2 = Tandang)
- `IB` = Menit Kejadian (contoh: `13'`)
- `IF` = Nama Pemain
- `IK` = Jenis Kejadian (contoh: `Goal`, `Yellow Card`, `Penalty Awarded`)

### Lineups (`df_li_1`)
- `LA` / `LB` = Header seksi (Starting Lineups / Substitutes)
- `LC` = Sisi Tim (1 = Kandang, 2 = Tandang)
- `LD` = Formasi (contoh: `1-4-4-2`)
- `LI` = Nama Pemain
- `LJ` = Nomor Punggung
- `LQ` = Kewarganegaraan

---

## 5. Keamanan & Rate Limiting (Penting!)
Sistem Flashscore mendeteksi volume *request* yang tidak wajar. Mengingat satu musim berisi ±306 pertandingan (yang berarti ~918 *request* detail), **wajib hukumnya** memberikan jeda:

```go
// Contoh di Golang
time.Sleep(1 * time.Second)
```
Tanpa waktu tunda (*delay*), IP server akan diblokir seketika oleh jaringan Cloudflare/Fastly. Data detail cukup disinkronkan hanya untuk pertandingan yang baru saja selesai (*status change*), jangan me-*loop* semua sejarah pertandingan di setiap *cron job*.
