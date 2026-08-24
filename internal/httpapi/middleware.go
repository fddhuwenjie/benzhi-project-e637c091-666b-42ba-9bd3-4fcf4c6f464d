package httpapi

import (
	"fmt"
	"net/http"
	"time"
)

func (h *Handler) requestMetadata(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'")
		w.Header().Set("X-Request-ID", fmt.Sprintf("req-%d", time.Now().UnixNano()))
		// 单机部署暂不接身份系统；该响应头明确当前鉴权边界，业务命令仍记录操作者。
		w.Header().Set("X-Auth-Mode", "operator-asserted")
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				writeError(w, fmt.Errorf("handler panic: %v", recovered))
			}
		}()
		next.ServeHTTP(w, r)
	})
}
