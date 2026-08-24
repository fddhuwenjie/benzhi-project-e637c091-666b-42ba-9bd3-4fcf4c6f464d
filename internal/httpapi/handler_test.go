package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"showcaseguard/internal/domain"
	"showcaseguard/internal/store"
	"showcaseguard/internal/workflow"
)

func testHandler(t *testing.T) http.Handler {
	t.Helper()
	storage, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	err = storage.SeedShowcases([]domain.Showcase{{ID: "case-1", Name: "测试柜", GalleryZone: "A", CollectionLevel: domain.CollectionGeneral, TargetTemperatureMin: 18, TargetTemperatureMax: 22, TargetHumidityMin: 45, TargetHumidityMax: 55, Active: true}})
	if err != nil {
		t.Fatal(err)
	}
	fallback := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	return New(workflow.New(storage), fallback).Routes()
}

func TestCreateAndConflictMapping(t *testing.T) {
	handler := testHandler(t)
	payload := map[string]any{"showcase_id": "case-1", "detected_at": time.Now().UTC(), "source": "manual", "metric": "humidity", "observed_value": 70, "reporter": "测试员", "description": "异常"}
	body, _ := json.Marshal(payload)
	request := httptest.NewRequest(http.MethodPost, "/api/incidents", bytes.NewReader(body))
	request.Header.Set("Idempotency-Key", "http-create-0001")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("创建状态码=%d body=%s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodPost, "/api/incidents", bytes.NewReader([]byte(`{"showcase_id":"case-1"}`)))
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("缺少幂等键应为 400，得到 %d", response.Code)
	}
}

func TestHealthAndNotFound(t *testing.T) {
	handler := testHandler(t)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if response.Code != http.StatusOK {
		t.Fatal(response.Code)
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/incidents/missing", nil))
	if response.Code != http.StatusNotFound {
		t.Fatal(response.Code)
	}
}
