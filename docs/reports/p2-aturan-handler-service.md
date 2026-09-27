# Laporan Pembuatan Aturan Handler ke Service

Tanggal: 2026-09-27

## Item yang dikerjakan
| ID | Masalah/Kebutuhan | Perbaikan/Implementasi | File |
|---|---|---|---|
| 1 | Kebutuhan dokumentasi standar arsitektur dari Handler ke Service | Membuat dokumen panduan/aturan pemisahan tanggung jawab (Separation of Concerns) antara HTTP Handler dan Business Logic Service berdasarkan implementasi yang sudah ada (contoh: di module product). | `.agents/rules/handler_to_service.md` |

## Test dan Verifikasi
- Aturan telah direview dan disesuaikan dengan pola eksisting pada file `internal/product/handler.go` dan `internal/product/service.go`.

## Temuan Baru
- Proyek menggunakan pattern Data Transfer Object (DTO) untuk Request dan Response di layer Handler.
- Error handling sudah tersentralisasi menggunakan `apperror` dan global error handler di router.
- Tracing menggunakan OpenTelemetry (`trace.Tracer`) wajib diimplementasikan di layer Service.

## Item Terbuka
- Tidak ada.
