package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Andreyka-coder9192/calc_go/internal/application"
)

func TestCalculateHandler(t *testing.T) {
	orch := application.NewOrchestrator()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orch.CalculateHandler(w, r)
	})

	reqBody := `{"expression": "(1+2)*3"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/calculate", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("Ожидался статус %d, получен %d", http.StatusCreated, rr.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("Ошибка декодирования ответа: %v", err)
	}
	if id, ok := resp["id"]; !ok || id == "" {
		t.Errorf("Ожидался валидный id в ответе, получено: %v", resp)
	}
}
