package httpx

import (
	"encoding/json"
	"errors"
	"github.com/d0nedev/newsscore/internal/platform/apperror"
	"net/http"
)

type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func WriteError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	code := apperror.CodeInternal
	message := "internal server error"

	var appErr *apperror.Error
	if errors.As(err, &appErr) {
		status = appErr.Status
		code = appErr.Code
		message = appErr.Message
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	_ = json.NewEncoder(w).Encode(
		ErrorResponse{
			Error: ErrorDetail{
				Code:    code,
				Message: message,
			},
		})

}

func WriteInternalServerError(w http.ResponseWriter) {
	WriteError(
		w,
		apperror.New(
			http.StatusInternalServerError,
			apperror.CodeInternal,
			"internal server error",
		),
	)
}
