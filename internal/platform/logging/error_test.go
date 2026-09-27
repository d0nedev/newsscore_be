package logging

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/d0nedev/newsscore/internal/platform/apperror"
)

func TestLogErrorLevelByStatus(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantLevel string
	}{
		{"client error", apperror.Validation("bad"), `"level":"WARN"`},
		{"server error", apperror.New(http.StatusInternalServerError, "X", "x"), `"level":"ERROR"`},
		{"unknown error", errors.New("boom"), `"level":"ERROR"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			LogError(slog.New(slog.NewJSONHandler(&buf, nil)), httptest.NewRequest(http.MethodGet, "/", nil), tt.err)

			if !strings.Contains(buf.String(), tt.wantLevel) {
				t.Errorf("log = %s, want %s", buf.String(), tt.wantLevel)
			}
		})
	}
}
