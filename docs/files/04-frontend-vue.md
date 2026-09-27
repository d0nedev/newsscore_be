# 04 — Frontend (Vue / Nuxt)

## 1. Stack

| Kebutuhan | Pilihan | Catatan |
|---|---|---|
| Kerangka kerja | Nuxt 4 (Vue 3.5) | SSR wajib, lihat keputusan 2.6 pada dokumen arsitektur |
| Bahasa | TypeScript, mode strict | |
| Gaya | Tailwind CSS v4 | Token didefinisikan sebagai CSS variable, bukan konfigurasi JS |
| State | Pinia | Hanya untuk state lintas halaman |
| Utilitas | VueUse | Ambil per fungsi, jangan impor keseluruhan |
| i18n | `@nuxtjs/i18n` | `id` utama, `en` cadangan |
| SEO | `@nuxtjs/seo` | Sitemap, robots, schema.org |
| Gambar | `@nuxt/image` | Logo klub dan sampul berita |
| Uji komponen | Vitest + Testing Library | |
| Uji ujung ke ujung | Playwright | Alur kritis saja |

**Yang sengaja tidak dipakai:**

- Pustaka komponen siap pakai. Halaman inti seluruhnya adalah tampilan khusus
  domain — baris pertandingan, tabel klasemen, kronologi kejadian. Pustaka
  umum tidak membantu di sana, tetapi tetap membawa beban ukuran berkas dan
  gaya bawaan yang justru harus dilawan.
- Pustaka tanggal seperti `date-fns` atau `dayjs`. `Intl.DateTimeFormat` sudah
  cukup, mendukung lokal `id-ID`, dan tidak menambah satu byte pun.
- Pustaka animasi. Hanya ada satu momen bergerak di seluruh situs, dan CSS
  sudah memadai untuk itu.

## 2. Struktur proyek

```
app/
├── assets/css/
│   ├── tokens.css            # sumber tunggal token desain
│   └── main.css
├── components/
│   ├── match/
│   │   ├── MatchRow.vue      # unit paling sering muncul, optimalkan lebih dulu
│   │   ├── MatchScore.vue
│   │   ├── MatchTimeline.vue
│   │   ├── MatchLineup.vue
│   │   └── MatchStats.vue
│   ├── league/
│   │   ├── StandingsTable.vue
│   │   └── LeagueTabs.vue
│   ├── news/
│   │   ├── ArticleCard.vue
│   │   └── ArticleBody.vue
│   ├── team/
│   └── ui/                   # primitif bersama, jumlahnya harus tetap kecil
├── composables/
│   ├── useApi.ts
│   ├── useLiveMatches.ts     # klien SSE
│   ├── useMatchDate.ts
│   └── useSeo.ts
├── layouts/
├── pages/
├── stores/
│   ├── favourites.ts         # disimpan di localStorage
│   └── preferences.ts
├── utils/
│   ├── format.ts
│   └── slug.ts
└── app.vue
shared/
└── types/api.ts              # tipe respons, dihasilkan dari OpenAPI
server/
└── api/                      # hanya bila perlu menyembunyikan kunci
```

Tipe di `shared/types/api.ts` sebaiknya dihasilkan dari spesifikasi OpenAPI yang
diterbitkan backend, bukan ditulis manual. Tipe yang ditulis dua kali akan
berbeda dalam hitungan pekan.

## 3. Routing

URL berbahasa Indonesia. Ini keputusan SEO, bukan selera.

| Jalur | Halaman |
|---|---|
| `/` | Beranda: pertandingan hari ini dan berita terbaru |
| `/pertandingan` | Alihkan ke tanggal hari ini |
| `/pertandingan/[tanggal]` | `2026-09-14` |
| `/pertandingan/[slug]` | `persib-vs-persija-12345` |
| `/liga/[slug]` | Ikhtisar kompetisi |
| `/liga/[slug]/klasemen` | |
| `/liga/[slug]/jadwal` | |
| `/tim/[slug]` | |
| `/pemain/[slug]` | |
| `/berita` | |
| `/berita/[slug]` | |
| `/cari` | |

Slug pertandingan menyertakan id di akhir agar pencocokan tetap pasti meski
nama klub berubah. Slug lama tetap dialihkan secara permanen ke slug baru —
tautan yang mati adalah peringkat pencarian yang hilang.

## 4. Strategi rendering

Inilah bagian yang paling menentukan apakah sasaran performa dan SEO tercapai.

```ts
// nuxt.config.ts
routeRules: {
  '/':                      { swr: 60 },
  '/pertandingan/**':       { swr: 30 },
  '/liga/**/klasemen':      { swr: 300 },
  '/tim/**':                { swr: 600 },
  '/pemain/**':             { swr: 3600 },
  '/berita':                { swr: 120 },
  '/berita/**':             { swr: 600 },
  '/cari':                  { ssr: true, headers: { 'cache-control': 'no-store' } },
  '/admin/**':              { ssr: false },
}
```

Prinsipnya:

- Setiap halaman publik dirender di server, lalu disimpan di cache Nitro.
  Pengunjung dari Google menerima HTML jadi tanpa menunggu JavaScript.
- Skor langsung tiba lewat SSE setelah hidrasi. HTML awal memuat skor pada saat
  permintaan; SSE memperbaruinya sejak itu. Tidak ada pemuatan ulang halaman.
- Panel admin sepenuhnya di sisi klien. Tidak butuh SEO dan tidak boleh
  menambah beban bundel halaman publik.

## 5. Pembaruan langsung

```ts
// app/composables/useLiveMatches.ts

export function useLiveMatches(date: string) {
  const matches = useState<Match[]>(`matches:${date}`, () => [])

  onMounted(() => {
    // Hanya sambungkan bila ada pertandingan yang berpotensi berjalan.
    if (!matches.value.some(isLiveOrUpcomingSoon)) return

    const source = new EventSource(`${apiBase}/v1/stream/matches?date=${date}`)

    source.addEventListener('score', (e) => {
      const patch = JSON.parse(e.data)
      applyScorePatch(matches, patch)
    })

    onScopeDispose(() => source.close())
  })

  return { matches }
}
```

Empat aturan:

1. **Jangan pernah menyambung di server.** `EventSource` hanya ada di peramban.
2. **Jangan menyambung bila tidak perlu.** Halaman yang menampilkan pertandingan
   pekan lalu tidak membuka koneksi apa pun.
3. **Selalu tutup koneksi saat komponen dilepas.** Kebocoran di sini akan
   menumpuk seiring navigasi pengguna.
4. **Terapkan patch, jangan ganti seluruh larik.** Mengganti larik memicu
   render ulang seluruh daftar dan menghapus posisi gulir.

Untuk aksesibilitas, perubahan skor diumumkan lewat `aria-live="polite"` pada
wadah skor. Pembaca layar akan menyampaikan perubahannya tanpa memotong
pembacaan yang sedang berjalan.

## 6. Anggaran performa

Mengikat dan diperiksa di CI lewat Lighthouse pada setiap pull request.

| Metrik | Anggaran | Halaman uji |
|---|---|---|
| LCP | 2,5 detik | `/pertandingan/[tanggal]` pada Slow 4G |
| CLS | 0,1 | seluruh halaman |
| TBT | 200 ms | `/pertandingan/[slug]` |
| JS awal | 180 KB terkompresi | seluruh halaman publik |
| Total permintaan | 40 | halaman pertandingan |

Cara mempertahankannya:

- **Logo klub sebagai SVG** bila tersedia, dengan `width` dan `height`
  eksplisit. Logo tanpa dimensi adalah penyebab CLS paling umum di situs skor.
- **Muat lambat di bawah lipatan.** Statistik, susunan pemain, dan rekam
  pertemuan pada halaman pertandingan dimuat saat tabnya dibuka, bukan di awal.
- **Satu keluarga huruf, sub-set Latin saja,** dengan `font-display: swap` dan
  pramuat pada berkas huruf utama.
- **Tanpa pustaka bagan pada MVP.** Kalau nanti dibutuhkan visualisasi,
  gambarkan sebagai SVG polos.
- **Periksa ukuran bundel di CI.** Kenaikan di atas 10 KB memerlukan alasan
  tertulis di deskripsi pull request.

## 7. SEO

Tanpa ini, seluruh keputusan arsitektur sebelumnya kehilangan maknanya.

**Data terstruktur per jenis halaman:**

| Halaman | Skema |
|---|---|
| Pertandingan | `SportsEvent` dengan `homeTeam`, `awayTeam`, `startDate`, `location` |
| Klasemen | `Table` di dalam `SportsOrganization` |
| Artikel | `NewsArticle` dengan `datePublished`, `author`, `image` |
| Klub | `SportsTeam` |
| Seluruh halaman | `BreadcrumbList` |

**Yang wajib ada pada setiap halaman:**

- `<title>` unik dan deskriptif, memuat nama entitas. Untuk pertandingan:
  nama kedua klub, kompetisi, dan tanggal.
- Meta description yang ditulis untuk manusia, bukan tempelan kata kunci.
- Tautan kanonis. Halaman pertandingan yang sama tidak boleh dapat diakses
  lewat dua URL berbeda.
- `hreflang` antara varian `id` dan `en`.
- Gambar Open Graph. Untuk pertandingan, buat gambar skor secara dinamis lewat
  rute di `server/`.

**Sitemap** dibangkitkan dari API, dipisah per jenis, dan disegarkan tiap jam
untuk pertandingan.

## 8. Aksesibilitas

Setara dengan anggaran performa, bukan pekerjaan tambahan di akhir.

- Kontras memenuhi WCAG AA. Ini memengaruhi pilihan warna pada dokumen desain,
  terutama warna penanda pertandingan berlangsung.
- Fokus papan ketik terlihat jelas pada seluruh elemen interaktif. Jangan
  menghapus `outline` tanpa menggantinya.
- Tabel klasemen memakai penanda tabel sungguhan dengan `<th scope>`, bukan
  susunan `div`.
- `prefers-reduced-motion` dihormati.
- Seluruh keadaan disampaikan lewat lebih dari sekadar warna. Zona degradasi
  pada klasemen ditandai dengan warna dan penanda tekstual sekaligus.

## 9. Penanganan keadaan kosong dan galat

Ditulis di sini agar konsisten, bukan diimprovisasi per komponen.

| Keadaan | Perlakuan |
|---|---|
| Tidak ada pertandingan pada tanggal itu | Sampaikan apa adanya, sertakan tautan ke tanggal terdekat yang ada pertandingannya |
| Data tertunda karena gangguan hulu | Tampilkan data terakhir disertai waktu pembaruan, jangan tampilkan halaman galat |
| Koneksi SSE terputus | Sambung ulang diam-diam. Beri tahu pengguna hanya setelah tiga kegagalan beruntun |
| Pencarian tanpa hasil | Sarankan kompetisi atau klub populer |
| Galat 500 | Jelaskan apa yang terjadi dan sediakan jalan keluar. Jangan meminta maaf berulang kali |

Pesan ditulis dengan kalimat aktif dan ringkas. Antarmuka menjelaskan apa yang
terjadi dan apa yang bisa dilakukan, bukan menyampaikan suasana hati.
