package httpx

import (
	"encoding/json"
	"errors"
	"github.com/d0nedev/newsscore/internal/platform/apperror"
	"io"
	"net/http"
)

const MaxRequestBodySize = 1 << 20

func DecodeJSON(
	w http.ResponseWriter,
	r *http.Request,
	dst any,
) error {
	r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBodySize)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		var maxBytesErr *http.MaxBytesError

		switch {
		case errors.As(err, &maxBytesErr):
			return apperror.New(
				http.StatusRequestEntityTooLarge,
				apperror.CodeRequestBodyTooLarge,
				"request body is too large",
			)
		case errors.Is(err, io.EOF):
			return apperror.Validation("request body is required")
		default:
			return apperror.Validation("invalid JSON body")
		}
	}

	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return apperror.Validation("request body must contain a single JSON object")
	}

	return nil
}
