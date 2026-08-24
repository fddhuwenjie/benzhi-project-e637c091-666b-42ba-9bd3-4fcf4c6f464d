package httpapi

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"showcaseguard/internal/domain"
	"showcaseguard/internal/store"
	"showcaseguard/internal/workflow"
)

type Handler struct {
	service *workflow.Service
	web     http.Handler
}

func New(service *workflow.Service, webHandler http.Handler) *Handler {
	return &Handler{service: service, web: webHandler}
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.Health)
	mux.HandleFunc("GET /api/showcases", h.ListShowcases)
	mux.HandleFunc("GET /api/incidents", h.ListIncidents)
	mux.HandleFunc("POST /api/incidents", h.CreateIncident)
	mux.HandleFunc("GET /api/incidents/{id}", h.GetIncident)
	mux.HandleFunc("POST /api/incidents/{id}/plans", h.SubmitPlan)
	mux.HandleFunc("POST /api/incidents/{id}/plans/approval", h.ApprovePlan)
	mux.HandleFunc("POST /api/incidents/{id}/actions", h.RecordAction)
	mux.HandleFunc("POST /api/incidents/{id}/verifications", h.RecordVerification)
	mux.HandleFunc("GET /api/archives", h.SearchArchives)
	mux.Handle("/", h.web)
	return h.recoverPanic(h.requestMetadata(mux))
}

func (h *Handler) Health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, envelope{Data: map[string]string{"status": "ok", "service": "showcaseguard"}})
}

func (h *Handler) ListShowcases(w http.ResponseWriter, _ *http.Request) {
	items := h.service.Showcases()
	writeJSON(w, http.StatusOK, envelope{Data: items, Meta: map[string]int{"count": len(items)}})
}

func (h *Handler) ListIncidents(w http.ResponseWriter, r *http.Request) {
	filter := workflow.IncidentFilter{ShowcaseID: r.URL.Query().Get("showcase_id"), State: domain.IncidentState(r.URL.Query().Get("state"))}
	items := h.service.Queue(filter)
	writeJSON(w, http.StatusOK, envelope{Data: items, Meta: map[string]int{"count": len(items)}})
}

func (h *Handler) CreateIncident(w http.ResponseWriter, r *http.Request) {
	key, err := idempotencyKey(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var command workflow.CreateIncidentCommand
	if err := decodeJSON(w, r, &command); err != nil {
		writeError(w, err)
		return
	}
	detail, created, err := h.service.CreateIncident(command, key)
	if err != nil {
		writeError(w, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	meta := map[string]any{"created": created, "aggregated": !created, "report_count": len(detail.Reports)}
	writeJSON(w, status, envelope{Data: detail, Meta: meta})
}

func (h *Handler) GetIncident(w http.ResponseWriter, r *http.Request) {
	detail, err := h.service.Detail(r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, envelope{Data: detail})
}

func (h *Handler) SubmitPlan(w http.ResponseWriter, r *http.Request) {
	key, err := idempotencyKey(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var command workflow.SubmitPlanCommand
	if err := decodeJSON(w, r, &command); err != nil {
		writeError(w, err)
		return
	}
	command.IncidentID = r.PathValue("id")
	detail, err := h.service.SubmitPlan(command, key)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, envelope{Data: detail})
}

func (h *Handler) ApprovePlan(w http.ResponseWriter, r *http.Request) {
	key, err := idempotencyKey(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var command workflow.ApprovePlanCommand
	if err := decodeJSON(w, r, &command); err != nil {
		writeError(w, err)
		return
	}
	command.IncidentID = r.PathValue("id")
	detail, err := h.service.ApprovePlan(command, key)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, envelope{Data: detail})
}

func (h *Handler) RecordAction(w http.ResponseWriter, r *http.Request) {
	key, err := idempotencyKey(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var command workflow.RecordActionCommand
	if err := decodeJSON(w, r, &command); err != nil {
		writeError(w, err)
		return
	}
	command.IncidentID = r.PathValue("id")
	detail, err := h.service.RecordAction(command, key)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, envelope{Data: detail})
}

func (h *Handler) RecordVerification(w http.ResponseWriter, r *http.Request) {
	key, err := idempotencyKey(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var command workflow.RecordVerificationCommand
	if err := decodeJSON(w, r, &command); err != nil {
		writeError(w, err)
		return
	}
	command.IncidentID = r.PathValue("id")
	detail, err := h.service.RecordVerification(command, key)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, envelope{Data: detail})
}

func (h *Handler) SearchArchives(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	zone := query.Get("gallery_zone")
	if zone == "" {
		zone = query.Get("zone")
	}
	q := store.ArchiveQuery{Query: query.Get("q"), GalleryZone: zone, Owner: query.Get("owner"), Severity: domain.Severity(query.Get("severity")), Limit: 20, Cursor: query.Get("cursor")}
	if raw := query.Get("limit"); raw != "" {
		if _, err := fmt.Sscanf(raw, "%d", &q.Limit); err != nil || q.Limit < 1 || q.Limit > 100 {
			writeError(w, domain.Validation("查询条件无效", map[string]string{"limit": "必须为1-100"}))
			return
		}
	}
	if q.Severity != "" {
		switch q.Severity {
		case domain.SeverityLow, domain.SeverityMedium, domain.SeverityHigh, domain.SeverityCritical:
		default:
			writeError(w, domain.Validation("查询条件无效", map[string]string{"severity": "严重度枚举无效"}))
			return
		}
	}
	parse := func(name string) *time.Time {
		if v := query.Get(name); v != "" {
			dateOnly := !strings.Contains(v, "T")
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				t, err = time.Parse("2006-01-02", v)
				if err == nil && (name == "closed_to" || name == "to") && dateOnly {
					t = t.Add(24*time.Hour - time.Nanosecond)
				}
			}
			if err != nil {
				writeError(w, domain.Validation("查询条件无效", map[string]string{name: "必须为日期或RFC3339时间"}))
				return nil
			}
			return &t
		}
		return nil
	}
	fromName, toName := "closed_from", "closed_to"
	if query.Get(fromName) == "" && query.Get("from") != "" {
		fromName = "from"
	}
	if query.Get(toName) == "" && query.Get("to") != "" {
		toName = "to"
	}
	q.From, q.To = parse(fromName), parse(toName)
	if (query.Get("closed_from") != "" && q.From == nil) || (query.Get("closed_to") != "" && q.To == nil) {
		return
	}
	if q.From != nil && q.To != nil && q.To.Before(*q.From) {
		writeError(w, domain.Validation("查询条件无效", map[string]string{"closed_to": "不得早于closed_from"}))
		return
	}
	if raw := query.Get("overdue"); raw != "" {
		v := raw == "true"
		if raw != "true" && raw != "false" {
			writeError(w, domain.Validation("查询条件无效", map[string]string{"overdue": "必须为true或false"}))
			return
		}
		q.Overdue = &v
	}
	result := h.service.SearchArchives(q)
	meta := map[string]any{"count": len(result.Items), "total": result.Total, "next_cursor": result.NextCursor, "stats": map[string]any{"average_duration_hours": result.AvgDuration, "max_duration_hours": result.MaxDuration, "overdue_count": result.OverdueCount, "average_action_count": result.AvgActions}}
	writeJSON(w, http.StatusOK, envelope{Data: result.Items, Meta: meta})
}
