# Aturan Handler ke Service (Go Backend)

Standar pemisahan tanggung jawab antara `Handler` (HTTP layer) dan `Service` (business logic layer).
Harus konsisten dengan `docs/api-contract-plan.md` (format response/error) dan `docs/postgres-schema-plan.md`.

Alur singkat: **Middleware → Handler → Service → SQLC (`db.Queries`) → PostgreSQL**, error kembali ke atas dan diubah jadi JSON di satu tempat.

---

## 0. Tipe Handler yang Mengembalikan Error

`http.HandlerFunc` bawaan tidak mengembalikan error. Semua handler memakai adapter ini:

```go
// httpx/handler.go
type HandlerFunc func(w http.ResponseWriter, r *http.Request) error

func (h HandlerFunc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := h(w, r); err != nil {
		WriteError(w, r, err) // satu-satunya tempat error → JSON
	}
}
```

Registrasi route: `r.Method(http.MethodGet, "/matches/{id}", httpx.HandlerFunc(h.GetMatch))`.

---

## 1. Middleware (urutan dari luar)

1. **RequestID:** baca/generate id, simpan di context, set header `X-Request-Id` di setiap response (FE membacanya).
2. **Recover:** panic → `apperror.Internal`, ditulis lewat `WriteError`.
3. **CORS:** `Access-Control-Allow-Origin` = origin FE persis (bukan `*`), `Allow-Credentials: true`, `Expose-Headers: X-Request-Id`.
4. **Timeout:** `http.TimeoutHandler` / context deadline global (mis. 10 detik). Service tidak memasang timeout sendiri.
5. **Session (opsional per route):** baca cookie sesi, cari `sessions` by `sha256(token)`, taruh `userID` di context. Route yang wajib login dibungkus `RequireAuth` → `apperror.Unauthorized` bila tidak ada.

---

## 2. Tanggung Jawab Handler (HTTP Layer)

Handler **hanya** urusan HTTP:

- **Parameter:** path (`chi.URLParam(r, "id")`) dan query string (`r.URL.Query()`).
- **Decode body:** `httpx.DecodeJSON` ke Request DTO; body rusak/unknown field → `apperror.Validation`.
- **Validasi input:** `req.Validate()` mengembalikan `apperror.Validation` dengan `Fields map[string]string` (key = nama field JSON).
- **Pagination:** decode cursor (base64 opaque). Cursor tidak valid → `400`. `limit` default 20, maksimum 100. Service menerima cursor yang sudah di-decode (struct), bukan string.
- **Auth:** ambil `userID` dari context (`auth.UserID(r.Context())`) lalu kirim sebagai **parameter eksplisit** ke service.
- **Panggil service:** selalu `r.Context()` sebagai parameter pertama.
- **Mapping ke Response DTO** (`toMatchResponse(...)`). Semua formatting tampilan ada di sini:
  - slug diekspos sebagai `id`;
  - tanggal `"DD.MM.YYYY"` / `"DD.MM.YY"` / `"DD.MM"` zona `Asia/Jakarta`;
  - `Match.time` = `"67'"` saat live, `"HH:MM"` selain itu;
  - label relatif berita (`"2 jam lalu"`), nilai pasar (`"€14,0 jt"`);
  - field opsional kosong → dihilangkan (`omitempty`).
- **Tulis response:** `httpx.WriteJSON(w, status, httpx.Data(resp))` → `{ "data": ... }`, atau `httpx.Page(items, nextCursor)` → `{ "data": [...], "meta": { "nextCursor": ... } }`. Status: `200`, `201`, `204` (tanpa body).
- **Error:** `return err` apa adanya. Handler tidak membungkus, tidak log, tidak menulis JSON error.

Handler **tidak boleh**: query DB, memanggil `db.Queries`, membuka transaksi, atau berisi aturan bisnis.

---

## 3. Tanggung Jawab Service (Business Logic Layer)

- **Konstruktor:** `NewService(pool *pgxpool.Pool, queries *db.Queries, tracer trace.Tracer)`. `pool` hanya dipakai untuk transaksi.
- **Tracing:** awal setiap method public:
  ```go
  ctx, span := s.tracer.Start(ctx, "MatchService.Get")
  defer span.End()
  ```
  Setiap jalur return error lewat `return tracing.Fail(span, err)` (record error + set status span).
- **Pemrosesan data:** konversi DTO ke tipe DB (`pgtype.Numeric`, `pgtype.Timestamptz`, dll.), aturan bisnis.
- **Database:** hanya lewat `s.queries` (SQLC). Operasi multi-tabel yang harus atomik memakai transaksi:
  ```go
  tx, err := s.pool.Begin(ctx)
  if err != nil { return tracing.Fail(span, apperror.Internal(err)) }
  defer tx.Rollback(ctx) // no-op setelah Commit
  q := s.queries.WithTx(tx)
  // ... q.UpdateStandings(ctx, ...), q.UpsertPlayerSeasonStats(ctx, ...)
  return tx.Commit(ctx)
  ```
  Contoh wajib transaksi: match `finished` → update `standings` + `player_season_stats`; login → buat `sessions`.
- **Return value:**
  - Endpoint satu tabel → struct SQLC asli (`db.Team`, `db.News`).
  - Endpoint gabungan → struct domain di package service, mis.:
    ```go
    type MatchDetail struct {
        Match      db.GetMatchRow
        Events     []db.MatchEvent
        Stats      []db.MatchStat
        Lineups    []db.MatchLineup
        Odds       []db.MatchOdd
        HeadToHead []db.HeadToHeadRow
        // ...
    }
    ```
  - **Tidak pernah** Response DTO atau apa pun dari package `httpx`.
- **Tidak tahu HTTP:** service tidak import `net/http`, tidak membaca cookie/header/context auth. Semua input lewat parameter.

---

## 4. Error

### `apperror` menyimpan jenis, bukan status HTTP

```go
type Kind int
const (
	KindValidation Kind = iota // 422 (400 untuk JSON/cursor rusak)
	KindUnauthorized           // 401
	KindForbidden              // 403
	KindNotFound               // 404
	KindConflict               // 409
	KindInternal               // 500
)

type Error struct {
	Kind    Kind
	Code    string            // "MATCH_NOT_FOUND", "EMAIL_TAKEN"
	Message string            // aman ditampilkan ke user
	Fields  map[string]string // hanya Validation
	Err     error             // penyebab asli, hanya untuk log
}
```

Status HTTP ditentukan **hanya** di `httpx.WriteError`.

### Mapping error DB (`mapError` di service)

| Error | apperror |
|---|---|
| `pgx.ErrNoRows` | `NotFound` |
| `pgconn.PgError` code `23505` (unique) | `Conflict` (mis. email sudah dipakai) |
| `23503` (foreign key) | `NotFound` bila entity referensi tidak ada (mis. pin liga yang tidak ada) |
| `23514` (check) | `Validation` |
| `context.Canceled` / `DeadlineExceeded` | dikembalikan apa adanya |
| lainnya | `Internal` |

### `httpx.WriteError`

Body sesuai kontrak FE (`app/utils/api-error.ts`):

```json
{ "error": { "code": "VALIDATION_FAILED", "message": "…", "details": { "email": "Email tidak valid" } } }
```

- `details` hanya untuk Validation, berupa map `field → string`.
- `Internal`: log `Err` asli + request id + path, kirim message generik `"Internal server error"`. Pesan/SQL asli **tidak pernah** dikirim ke client.
- `context.Canceled` (client putus): jangan log sebagai error, jangan tulis body.
- `DeadlineExceeded`: `503`, code `TIMEOUT`.

---

## 5. Contoh Alur: `GET /api/v1/matches/{id}`

```go
// handler
func (h *MatchHandler) GetMatch(w http.ResponseWriter, r *http.Request) error {
	slug := chi.URLParam(r, "id")
	m, err := h.service.Get(r.Context(), slug)
	if err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusOK, httpx.Data(toMatchResponse(m)))
}

// service
func (s *MatchService) Get(ctx context.Context, slug string) (MatchDetail, error) {
	ctx, span := s.tracer.Start(ctx, "MatchService.Get")
	defer span.End()

	row, err := s.queries.GetMatchBySlug(ctx, slug)
	if err != nil {
		return MatchDetail{}, tracing.Fail(span, mapError(err, "MATCH_NOT_FOUND"))
	}
	events, err := s.queries.ListMatchEvents(ctx, row.ID)
	if err != nil {
		return MatchDetail{}, tracing.Fail(span, mapError(err, ""))
	}
	// ... stats, lineups, odds, commentary, player stats, head-to-head
	return MatchDetail{Match: row, Events: events /* ... */}, nil
}
```

1. Request masuk, middleware set request id + timeout.
2. Handler ambil slug, panggil `service.Get(r.Context(), slug)`.
3. Service buka span, query SQLC, `ErrNoRows` → `apperror.NotFound("MATCH_NOT_FOUND")`.
4. Sukses: service kembalikan `MatchDetail`.
5. Handler mapping ke `MatchResponse` (format tanggal, `time`, slug sebagai id) → `200 { "data": ... }`.
6. Gagal: handler `return err` → `WriteError` → `404 { "error": { "code": "MATCH_NOT_FOUND", ... } }`.

## 6. Contoh Alur Tulis: `PUT /api/v1/me/pins/leagues/{id}`

```go
func (h *PinHandler) PinLeague(w http.ResponseWriter, r *http.Request) error {
	userID := auth.UserID(r.Context()) // dijamin ada oleh RequireAuth
	if err := h.service.PinLeague(r.Context(), userID, chi.URLParam(r, "id")); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
```

Service: cari competition by slug (`NotFound` bila tidak ada), lalu `INSERT ... ON CONFLICT DO NOTHING` (idempotent, tidak pernah `Conflict`).

---

## 7. Checklist Review PR

- [ ] Handler tidak import `db` / tidak memanggil `queries`.
- [ ] Service tidak import `net/http` / `httpx`.
- [ ] `r.Context()` diteruskan; service tidak membuat `context.Background()`.
- [ ] Setiap method service punya span + `tracing.Fail` di semua jalur error.
- [ ] Operasi multi-tabel memakai transaksi.
- [ ] Error dari service tidak dibungkus ulang di handler.
- [ ] Validasi mengembalikan `Fields` dengan nama field JSON.
- [ ] Formatting tanggal/label hanya di mapping response handler.
- [ ] Tidak ada interface untuk service yang implementasinya cuma satu (tambah saat test butuh mock).
