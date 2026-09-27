# Aturan Handler ke Service (Go Backend)

Dokumen ini mendefinisikan standar dan aturan pemisahan tanggung jawab (Separation of Concerns) antara `Handler` (HTTP layer) dan `Service` (Business Logic layer) di dalam repository ini.

## 1. Tanggung Jawab Handler (HTTP Layer)
Handler bertugas sebagai pintu masuk dari request HTTP. Tanggung jawabnya **hanya** berfokus pada urusan protokol HTTP:
- **Routing & Path Parameters:** Mengekstrak parameter dari URL (misal: `chi.URLParam(r, "id")`) dan Query Strings (`r.URL.Query()`).
- **Decoding Request (DTO):** Melakukan konversi dari HTTP JSON Body menjadi struct Request / DTO (Data Transfer Object) menggunakan helper seperti `httpx.DecodeJSON`.
- **Validasi Dasar:** Memanggil metode validasi dari struct Request (misal: `req.Validate()`).
- **Memanggil Service:** Meneruskan data yang sudah divalidasi dan **wajib** meneruskan `r.Context()` sebagai parameter pertama ke fungsi di Service.
- **Mapping ke Response (DTO):** Menerima hasil dari Service (berupa Entitas/Model DB) lalu mengonversinya menjadi Response DTO (misal dengan fungsi `toMatchResponse()`).
- **Menulis Response:** Mengirim HTTP Status Code yang sesuai (`200 OK`, `201 Created`, `204 No Content`) dan JSON Response menggunakan `httpx.WriteJSON`.
- **Tidak ada Business Logic:** Handler **tidak boleh** melakukan query ke database secara langsung atau memproses logika bisnis.

## 2. Tanggung Jawab Service (Business Logic Layer)
Service adalah tempat di mana aturan bisnis (business logic) dan interaksi dengan database terjadi:
- **Dependency Injection:** Service menerima dependensi utama seperti `db.Queries` dan `trace.Tracer` melalui constructor (`NewService`).
- **Context & Tracing:** Menerima `context.Context` dari Handler. Selalu mulai tracing span di awal fungsi: `ctx, span := s.tracer.Start(ctx, "NamaService.Metode")` dan panggil `defer span.End()`.
- **Pemrosesan Data:** Memproses DTO yang dikirim oleh Handler, mempersiapkan tipe data yang sesuai untuk database (contoh: konversi string ke `pgtype.Numeric`).
- **Interaksi Database:** Memanggil fungsi query dari `s.queries` (SQLC layer).
- **Error Handling & Mapping:** Mengonversi error dari database (misalnya `ErrNoRows`) menjadi standard `apperror` (contoh: `apperror.NotFound` atau `apperror.Internal`) menggunakan helper seperti `mapError` atau `tracing.Fail`.
- **Return Value:** Mengembalikan Domain Entity atau struct database asli (misal `db.Match`), bukan HTTP Response DTO.

## 3. Aturan Return Error
- **Service:** Mengembalikan error tipe `error` yang dibungkus oleh `apperror` (beserta detail status HTTP-nya jika memungkinkan).
- **Handler:** Jika menerima `err != nil` dari Service, Handler **cukup mengembalikan error tersebut secara langsung** (`return err`). Pengolahan error menjadi format JSON standar akan ditangani oleh middleware/error handler global di tingkat router HTTP.

## Contoh Alur Kerja (Workflow)
1. Request masuk ke `Handler.GetMatch`.
2. `Handler` memanggil `parseMatchID` untuk membaca `{id}` dari URL dan memvalidasinya.
3. `Handler` memanggil `h.service.Get(r.Context(), id)`.
4. `Service.Get` membuat trace span baru, memproses tipe data, dan memanggil `s.queries.GetMatch(ctx, ...)`.
5. Jika sukses, `Service` mengembalikan model `db.Match` ke `Handler`.
6. `Handler` membungkus model `db.Match` ke dalam `MatchResponse` menggunakan `toMatchResponse()`.
7. `Handler` memanggil `httpx.WriteJSON(w, http.StatusOK, response)`.
