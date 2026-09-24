# 05 — Arah Desain

## 1. Titik tolak

Subjeknya sepak bola Indonesia. Audiensnya pendukung klub yang membuka situs
ini di sela pekerjaan, di angkutan umum, atau sambil menonton. Tugas utamanya
satu: menjawab "berapa skornya" dalam satu detik pertama.

Ancaman desainnya jelas. Situs skor punya tampilan bawaan yang sangat kuat —
latar gelap kebiruan, tabel padat, huruf kecil, kartu membulat dengan bayangan
lembut, aksen oranye atau hijau menyala. SofaScore, FlashScore, dan hampir
semua pesaing berada di wilayah itu. Meniru berarti terlihat seperti tiruan
yang lebih miskin.

Arah yang diambil berangkat dari papan skor stadion dan budaya visual tribun
Indonesia: huruf tinggi memanjang, kontras keras, warna yang punya arti tetap
dalam sepak bola, dan tidak ada yang dekoratif.

## 2. Palet

```css
--pitch:    #123F2E;  /* hijau lapangan di bawah lampu sorot */
--chalk:    #EDEFE9;  /* putih garis lapangan, permukaan utama */
--ink:      #10160F;  /* teks, hitam bersemu hijau */
--sodium:   #F2B705;  /* kuning lampu sorot, aksen dan kartu kuning */
--signal:   #D62828;  /* merah, penanda berlangsung dan kartu merah */
--slate:    #5C6660;  /* teks sekunder, garis pemisah */
```

Alasan pemilihan, karena setiap warna harus bisa dipertanggungjawabkan:

**Terang sebagai bawaan.** Seluruh pesaing gelap. Permukaan terang langsung
membedakan, dan lebih terbaca di luar ruangan pada siang hari Jakarta — kondisi
pemakaian yang sebenarnya. Mode gelap tetap tersedia sebagai pilihan, bukan
sebagai bawaan.

**`--chalk` bukan krem hangat.** Nilainya sengaja bersemu dingin kehijauan,
bukan `#F4F1EA` yang kini menjadi ciri khas halaman buatan mesin. Rujukannya
kapur garis lapangan, bukan kertas.

**Merah, kuning, hijau punya arti tetap dalam sepak bola.** Di konteks lain
kombinasi ini terbaca seperti lampu lalu lintas dan sebaiknya dihindari. Di
sini justru sebaliknya: kuning dan merah sudah berarti kartu, hijau berarti
lapangan. Pengguna tidak perlu mempelajari kode warna apa pun.

**`--signal` hanya untuk keadaan berlangsung.** Tidak dipakai untuk tombol,
tautan, atau penekanan. Ketika satu warna hanya berarti satu hal, mata
menemukannya tanpa membaca.

Kontras `--ink` di atas `--chalk` berada di angka 14:1, jauh di atas ambang
WCAG AA. `--signal` di atas `--chalk` mencapai 4,8:1, memadai untuk teks
berukuran normal.

## 3. Tipografi

Satu keluarga huruf: **Archivo**, sebuah variable font dengan sumbu lebar.

Sumbu lebar itulah perangkat desain utamanya, dan itu langsung berasal dari
subjeknya — papan skor stadion memakai angka tinggi memanjang, sementara papan
pengumuman memakai huruf melebar.

| Peran | Pengaturan |
|---|---|
| Skor | Archivo Expanded 700, angka tabular, ukuran besar |
| Judul halaman | Archivo 600, jarak huruf sedikit dirapatkan |
| Teks isi | Archivo 400, tinggi baris 1,6 |
| Tabel klasemen | Archivo Condensed 500, angka tabular |
| Menit dan label | Archivo Condensed 600 |

Angka tabular bersifat wajib di mana pun angka berubah. Tanpa itu, skor akan
bergeser horizontal setiap kali gol terjadi, dan pergeseran itu terlihat seperti
kerusakan.

Skala tipe mengikuti rasio 1,25 dari 16 piksel: 16, 20, 25, 31, 39, 49.

Tiga hal yang tidak dilakukan, karena merupakan penanda halaman generik:
menekankan satu kata dalam judul dengan warna atau miring; memakai huruf kapital
seluruhnya untuk label; menambahkan label tipografis di atas konten yang sudah
jelas dengan sendirinya.

## 4. Tata letak

Unit yang paling sering dilihat adalah baris pertandingan. Ia muncul ratusan
kali per halaman, dan kualitas seluruh situs praktis ditentukan olehnya.

```
┌──────────────────────────────────────────────────────┐
│ Liga 1 · Pekan 12                                    │  ← kepala kompetisi
├──────────────────────────────────────────────────────┤
│ ▌67'  Persib Bandung              2                  │  ← pita kiri = berlangsung
│ ▌     Persija Jakarta             1                  │
├──────────────────────────────────────────────────────┤
│  19:00 Arema FC                   –                  │
│        Bali United                –                  │
├──────────────────────────────────────────────────────┤
│  FT    PSM Makassar               0                  │
│        Borneo FC                  3                  │
└──────────────────────────────────────────────────────┘
```

Keputusan pentingnya:

**Baris pertandingan bukan kartu.** Tidak ada bayangan, tidak ada sudut
membulat, tidak ada jarak antarbaris. Hanya garis pemisah setipis satu piksel.
Kartu yang diberi jarak menghabiskan ruang vertikal, dan di layar ponsel itu
berarti lebih sedikit pertandingan yang terlihat sekaligus. Papan skor stadion
juga tidak memberi bingkai pada tiap barisnya.

**Keadaan berlangsung ditandai pita vertikal di tepi kiri,** bukan badge, bukan
titik berkedip. Pita itu dapat dipindai sekilas ketika menggulir cepat, dan tidak
menambah lebar apa pun pada baris.

**Skor rata kanan, nama klub rata kiri.** Kolom angka yang lurus dapat dibaca
secara vertikal tanpa membaca nama klubnya.

**Kolom tunggal sampai 768 piksel, dua kolom di atasnya** dengan berita di
kolom kanan. Tanpa sidebar di ponsel.

Lebar baris teks artikel dibatasi 68 karakter.

## 5. Gerak

Satu momen saja di seluruh situs: **ketika skor berubah, angka baru muncul
dengan latar `--sodium` yang memudar selama 900 milidetik.**

Itu saja. Tidak ada animasi masuk pada setiap bagian, tidak ada transisi
mengambang pada setiap kartu, tidak ada pemuat berkilau. Gerak yang tersebar di
mana-mana justru membuat satu-satunya kejadian yang benar-benar penting menjadi
tidak terlihat.

Kilatan itu dihilangkan sepenuhnya ketika `prefers-reduced-motion` aktif, dan
perubahan skor tetap diumumkan lewat `aria-live`.

## 6. Ikonografi dan gambar

Ikon digambar sendiri sebagai SVG, dengan jumlah dijaga di bawah lima belas.
Ketebalan garis 1,5 piksel, ujung persegi mengikuti karakter tegas hurufnya.
Jangan memasang pustaka ikon untuk sepuluh bentuk.

Logo klub adalah satu-satunya gambar pada halaman skor, ditampilkan pada 24
piksel dengan dimensi eksplisit. Kalau logo gagal dimuat, tampilkan inisial klub
di atas `--slate`, bukan kotak kosong.

Sampul berita memakai rasio 16:9 dengan tempat penampung berwarna solid selama
pemuatan, agar tidak terjadi pergeseran tata letak.

## 7. Pemeriksaan terhadap kemiripan

Sebelum halaman apa pun dinyatakan selesai, tempatkan tangkapan layarnya
berdampingan dengan halaman setara di SofaScore dan periksa:

- Apakah struktur navigasinya berbeda? Harus berbeda.
- Apakah baris pertandingannya punya bentuk yang berbeda? Harus berbeda.
- Apakah ada ikon, tata letak, atau istilah yang terbawa? Tidak boleh ada.
- Kalau logo dilepas dari keduanya, apakah masih bisa dibedakan? Harus bisa.

Pertanyaan terakhir adalah yang paling menentukan.
