package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"showcaseguard/internal/domain"
)

type smokeClient struct {
	client  *http.Client
	baseURL string
	serial  int
}

type smokeEnvelope struct {
	Data json.RawMessage `json:"data"`
	Meta json.RawMessage `json:"meta"`
}

func (c *smokeClient) post(path, operation string, input, output any) error {
	body, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("编码 %s 请求: %w", operation, err)
	}
	c.serial++
	request, err := http.NewRequest(http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Idempotency-Key", fmt.Sprintf("self-check-%s-%d-%d", operation, time.Now().UnixNano(), c.serial))
	response, err := c.client.Do(request)
	if err != nil {
		return fmt.Errorf("调用 %s: %w", operation, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
		return fmt.Errorf("%s 返回 %d: %s", operation, response.StatusCode, string(message))
	}
	var envelope smokeEnvelope
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&envelope); err != nil {
		return fmt.Errorf("解析 %s 响应: %w", operation, err)
	}
	if output != nil {
		if err := json.Unmarshal(envelope.Data, output); err != nil {
			return fmt.Errorf("解析 %s 业务数据: %w", operation, err)
		}
	}
	return nil
}

func exerciseWorkflow(client *http.Client, baseURL string) error {
	smoke := &smokeClient{client: client, baseURL: baseURL}
	now := time.Now().UTC().Truncate(time.Second)
	create := map[string]any{
		"showcase_id": "SC-A01", "detected_at": now.Add(-time.Minute), "source": "sensor",
		"metric": "humidity", "observed_value": 72.5, "reporter": "self-check", "description": "自检生成的微环境偏差",
	}
	var detail domain.IncidentDetail
	if err := smoke.post("/api/incidents", "create", create, &detail); err != nil {
		return err
	}
	if detail.Incident.State != domain.StateAssessed || detail.Assessment == nil {
		return fmt.Errorf("登记后未完成影响评估")
	}
	incidentPath := "/api/incidents/" + url.PathEscape(detail.Incident.ID)
	plan := map[string]any{
		"expected_revision": detail.Incident.Revision, "owner": "self-check", "due_at": now.Add(time.Hour),
		"steps": []map[string]any{{"order": 1, "instruction": "检查展柜密封与调湿材料"}}, "risk_notes": "自检记录", "actor": "self-check",
	}
	if err := smoke.post(incidentPath+"/plans", "plan", plan, &detail); err != nil {
		return err
	}
	approval := map[string]any{"expected_revision": detail.Incident.Revision, "approver": "self-check-supervisor", "approved": true, "comment": "同意执行"}
	if err := smoke.post(incidentPath+"/plans/approval", "approval", approval, &detail); err != nil {
		return err
	}
	actionAt := time.Now().UTC()
	action := map[string]any{"expected_revision": detail.Incident.Revision, "performed_at": actionAt, "operator": "self-check", "description": "复位密封条并更换调湿材料", "evidence_ref": "self-check://evidence/1"}
	if err := smoke.post(incidentPath+"/actions", "action", action, &detail); err != nil {
		return err
	}
	verification := func(measuredAt time.Time) map[string]any {
		return map[string]any{"expected_revision": detail.Incident.Revision, "measured_at": measuredAt, "temperature": 20.0, "humidity": 50.0, "instrument_id": "SELF-CHECK-METER", "comment": "目标区间内", "operator": "self-check"}
	}
	if err := smoke.post(incidentPath+"/verifications", "verify-1", verification(actionAt.Add(time.Minute)), &detail); err != nil {
		return err
	}
	if detail.Incident.State != domain.StateVerifying {
		return fmt.Errorf("首次达标复测后状态为 %s", detail.Incident.State)
	}
	if err := smoke.post(incidentPath+"/verifications", "verify-2", verification(actionAt.Add(2*time.Minute)), &detail); err != nil {
		return err
	}
	if detail.Incident.State != domain.StateClosed || detail.Archive == nil {
		return fmt.Errorf("连续复测后未生成关闭档案")
	}
	response, err := client.Get(baseURL + "/api/archives?q=" + url.QueryEscape(detail.Incident.ID))
	if err != nil {
		return fmt.Errorf("检索档案: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("档案检索返回 %d", response.StatusCode)
	}
	var archiveEnvelope smokeEnvelope
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&archiveEnvelope); err != nil {
		return err
	}
	var archives []domain.ArchiveRecord
	if err := json.Unmarshal(archiveEnvelope.Data, &archives); err != nil || len(archives) != 1 {
		return fmt.Errorf("关闭档案不可检索")
	}
	return nil
}
