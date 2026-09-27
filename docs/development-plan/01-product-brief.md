# 01 — Product Brief

## 1. Masalah yang dipecahkan

Pendukung sepak bola Indonesia saat ini memakai dua alat terpisah: aplikasi
skor global untuk angka, dan portal berita lokal untuk konteks. Keduanya tidak
saling bicara. Platform global memperlakukan Liga 1 sebagai kompetisi baris
kesekian tanpa berita, tanpa profil pemain yang lengkap, dan tanpa bahasa yang
dipahami pembacanya. Portal berita lokal punya konteks tetapi skornya lambat
dan tidak dapat diandalkan.

LigaLive mengisi celah itu: kedalaman liga Indonesia, skor yang cepat, dan
berita yang menempel pada pertandingan yang sedang dibaca.

## 2. Pengguna

**Pengguna utama — pendukung klub.** Usia 18–35, mayoritas mengakses lewat
Android kelas menengah. Mengikuti satu klub Liga 1 secara intens dan satu klub
Eropa secara umum. Membuka situs beberapa kali dalam sehari saat klubnya
bertanding, dan sekali sehari di luar itu. Toleransi terhadap halaman lambat
sangat rendah.

**Pengguna sekunder — pencari hasil.** Datang dari Google dengan kueri spesifik
seperti hasil pertandingan tertentu atau klasemen terbaru. Tidak punya loyalitas
pada situs mana pun. Akan kembali hanya kalau halaman yang didarati memuat cepat
dan menjawab pertanyaannya di layar pertama tanpa perlu menggulir.

Pengguna sekunder inilah sumber pertumbuhan. Karena itu setiap halaman harus
berdiri sendiri sebagai halaman pendaratan, bukan sebagai cabang dari beranda.

## 3. Pembeda dari SofaScore

SofaScore adalah acuan kualitas, bukan model yang ditiru. Ia unggul sebagai
alat data global dan akan tetap unggul di wilayah itu. Bersaing langsung di
sana adalah pertarungan yang tidak bisa dimenangkan.

Empat sumbu pembedanya:

### 3.1 Kedalaman lokal, bukan keluasan global

SofaScore mencakup puluhan cabang olahraga di ratusan negara dengan kedalaman
merata dan tipis. LigaLive mencakup sedikit kompetisi dengan kedalaman penuh:

- **Prioritas pertama** — Liga 1, Liga 2, Piala Presiden, dan seluruh level
  tim nasional. Profil pemain, riwayat transfer, statistik musim, berita.
- **Prioritas kedua** — kompetisi yang paling ditonton dari Indonesia: Liga
  Inggris, Liga Champions, Liga Spanyol, Liga Italia. Cukup skor, klasemen,
  dan statistik dasar.
- **Tidak dicakup** — cabang olahraga selain sepak bola pada tahap ini.

Menolak cakupan luas adalah keputusan produk, bukan keterbatasan. Cakupan
sempit membuat kualitas per halaman bisa jauh melampaui platform global.

### 3.2 Berita menempel pada entitas, bukan berdiri terpisah

Di SofaScore, berita adalah kanal terpisah yang jarang dibuka. Di LigaLive,
setiap artikel ditautkan ke entitas yang dibahasnya — pertandingan, klub, atau
pemain. Konsekuensinya di antarmuka:

- Halaman pertandingan memuat pratinjau sebelum kickoff, laporan setelah
  peluit akhir, dan berita terkait di bawah statistik.
- Halaman klub memuat berita terbaru tentang klub itu di samping klasemen.
- Artikel memuat blok skor langsung ketika pertandingan yang dibahasnya sedang
  berjalan.

Secara teknis ini berarti relasi many-to-many antara artikel dan entitas, yang
skemanya ada di [`03-backend-go.md`](03-backend-go.md).

### 3.3 Kecepatan sebagai fitur

SofaScore versi web berat karena harus melayani cakupan global. LigaLive punya
anggaran performa yang mengikat dan diuji di CI:

| Metrik | Target | Diukur pada |
|---|---|---|
| Largest Contentful Paint | di bawah 2,5 detik | Slow 4G, Moto G Power |
| JavaScript per halaman | di bawah 180 KB terkompresi | halaman pertandingan |
| Cumulative Layout Shift | di bawah 0,1 | seluruh halaman |
| Time to First Byte | di bawah 400 ms | dari Jakarta |

Halaman yang melampaui anggaran tidak boleh masuk ke production. Ini mengikat,
bukan aspirasi.

### 3.4 Dibangun untuk pencarian berbahasa Indonesia

Seluruh URL berbahasa Indonesia (`/pertandingan/`, `/klasemen/`, `/berita/`),
seluruh konten dirender di server, dan setiap halaman membawa data terstruktur
yang sesuai. SofaScore memenangkan pertarungan aplikasi; ruang yang terbuka ada
di hasil pencarian berbahasa Indonesia, dan di sanalah upaya diarahkan.

### 3.5 Batas yang tidak dilewati

Ditulis eksplisit karena berkonsekuensi hukum:

- Tidak menyalin tata letak, ikonografi, atau alur navigasi SofaScore.
- Tidak mereplikasi fitur berhak milik mereka seperti sistem rating pemain atau
  grafik momentum serangan. Kalau dibutuhkan metrik sejenis, rancang sendiri
  dengan metodologi yang dipublikasikan terbuka.
- Data pertandingan diambil dari feed publik Flashscore (lihat
  [`07-scraping-flashscore.md`](../files/07-scraping-flashscore.md)). Tidak
  berlisensi: status hukum dan ketentuan layanan Flashscore wajib dikaji
  sebelum peluncuran, dan arsitektur harus siap berganti penyedia.
- Tidak menampilkan odds, prediksi berbayar, maupun tautan ke situs taruhan.

## 4. Lingkup MVP

### Skor dan pertandingan
- Daftar pertandingan per tanggal, terkelompok berdasarkan kompetisi
- Penanda status: akan datang, berlangsung dengan menit berjalan, selesai
- Pembaruan skor otomatis tanpa perlu memuat ulang halaman
- Halaman detail: susunan pemain, statistik, kronologi kejadian, rekam jejak
  pertemuan, dan berita terkait
- Klasemen kompetisi dengan penanda zona juara dan degradasi

### Berita
- Daftar berita dengan penyaringan berdasarkan kompetisi dan klub
- Halaman artikel dengan penulis, waktu terbit, dan entitas terkait
- Panel admin untuk menulis, menyunting, dan menerbitkan artikel

### Klub dan pemain
- Halaman klub: jadwal, hasil, skuad, klasemen, berita
- Halaman pemain: biodata, statistik musim berjalan, riwayat pertandingan

### Umum
- Pencarian klub, pemain, dan kompetisi
- Bahasa Indonesia sebagai bahasa utama, Inggris sebagai cadangan
- Tampilan terang dan gelap

## 5. Yang sengaja tidak dikerjakan pada MVP

Ditulis agar tidak masuk diam-diam di tengah pengerjaan.

| Tidak dikerjakan | Alasan |
|---|---|
| Akun pengguna dan login | Tidak ada fitur MVP yang membutuhkannya. Favorit cukup disimpan di perangkat. |
| Komentar dan forum | Butuh moderasi berkelanjutan yang belum ada sumber dayanya. |
| Aplikasi mobile | Sudah dikeluarkan dari lingkup atas keputusan klien. |
| Cabang olahraga selain sepak bola | Melanggar prinsip kedalaman di atas. |
| Siaran langsung dan video | Lisensi mahal dan berisiko hukum. |
| Odds dan prediksi | Lihat 3.5. |
| Notifikasi push web | Nilainya rendah tanpa aplikasi; tingkat izin di browser sangat kecil. |
| Statistik lanjutan buatan sendiri | Butuh validasi metodologi yang panjang. Pertimbangkan setelah trafik stabil. |

## 6. Ukuran keberhasilan

Enam bulan setelah peluncuran:

- Halaman klasemen dan hasil Liga 1 muncul di halaman pertama pencarian Google
  untuk kueri utama berbahasa Indonesia
- Lebih dari separuh sesi berasal dari pencarian organik
- Rata-rata LCP lapangan di bawah 2,5 detik pada persentil ke-75
- Lebih dari 1,5 halaman per sesi, yang menandakan berita dan skor benar-benar
  saling menyambung
