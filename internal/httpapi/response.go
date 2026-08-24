package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"showcaseguard/internal/domain"
	"showcaseguard/internal/workflow"
)

const maxRequestBody = 1 << 20

type envelope struct {
	Data  any       `json:"data,omitempty"`
	Error *apiError `json:"error,omitempty"`
	Meta  any       `json:"meta,omitempty"`
}

type apiError struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, value envelope) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, err error) {
	status, code := http.StatusInternalServerError, "internal_error"
	message := "服务暂时不可用"
	fields := map[string]string(nil)
	var problem *domain.Problem
	if errors.As(err, &problem) {
		message, fields = problem.Message, problem.Fields
	}
	switch {
	case errors.Is(err, domain.ErrNotFound):
		status, code, message = http.StatusNotFound, "not_found", "记录不存在"
	case workflow.IsConflict(err):
		status, code = http.StatusConflict, "conflict"
		if message == "服务暂时不可用" {
			message = err.Error()
		}
	case errors.Is(err, domain.ErrValidation):
		status, code = http.StatusBadRequest, "validation_error"
	case errors.Is(err, io.EOF):
		status, code, message = http.StatusBadRequest, "invalid_json", "请求体不能为空"
	}
	writeJSON(w, status, envelope{Error: &apiError{Code: code, Message: message, Fields: fields}})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	limited := http.MaxBytesReader(w, r.Body, maxRequestBody)
	decoder := json.NewDecoder(limited)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return domain.Validation("JSON 请求无效", map[string]string{"body": err.Error()})
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return domain.Validation("JSON 请求无效", map[string]string{"body": "只能包含一个 JSON 对象"})
	}
	return nil
}

func idempotencyKey(r *http.Request) (string, error) {
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		return "", domain.Validation("缺少 Idempotency-Key", map[string]string{"Idempotency-Key": "写请求必须提供幂等键"})
	}
	return key, nil
}
