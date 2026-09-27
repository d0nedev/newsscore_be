package database

import (
	"uuid"

	"github.com/jackc/pgx/v5/pgtype"
)

// UUID converts a parsed id into a query argument.
func UUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}
