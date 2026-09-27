package httpx

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/d0nedev/newsscore/internal/platform/apperror"
)

func TestWriteErrorUnwrapsAppError(t *testing.T) {
	err := fmt.Errorf("handler: %w", apperror.New(http.StatusNotFound, "NOT_FOUND", "missing"))

	rec := httptest.NewRecorder()
	WriteError(rec, err)

	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "NOT_FOUND") {
		t.Errorf("status = %d, body = %s", rec.Code, rec.Body)
	}
}

func TestWriteErrorHidesUnknownErrors(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, fmt.Errorf("pq: password=secret"))

	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "secret") {
		t.Errorf("status = %d, body = %s", rec.Code, rec.Body)
	}
}
