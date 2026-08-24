package store

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"showcaseguard/internal/domain"
)

// reconcileAuditLog 核对追加日志与快照。提交顺序是先替换快照、后追加日志，
// 因此进程在两者之间退出时，快照中的少量事件会在这里补写。
func (s *Store) reconcileAuditLog() error {
	snapshotEvents := make(map[string]domain.AuditEvent)
	for incidentID, events := range s.data.Audit {
		for _, event := range events {
			if event.ID == "" || event.IncidentID != incidentID {
				return fmt.Errorf("快照包含无效审计事件: incident=%s event=%s", incidentID, event.ID)
			}
			if _, duplicate := snapshotEvents[event.ID]; duplicate {
				return fmt.Errorf("快照包含重复审计事件: %s", event.ID)
			}
			snapshotEvents[event.ID] = event
		}
	}
	logged, err := s.readAuditLog()
	if err != nil {
		return err
	}
	for id := range logged {
		if _, exists := snapshotEvents[id]; !exists {
			return fmt.Errorf("审计日志事件 %s 不存在于当前快照，拒绝忽略审计数据", id)
		}
	}
	missing := make([]domain.AuditEvent, 0)
	for id, event := range snapshotEvents {
		if _, exists := logged[id]; !exists {
			missing = append(missing, event)
		}
	}
	sort.Slice(missing, func(i, j int) bool {
		if missing[i].At.Equal(missing[j].At) {
			return missing[i].ID < missing[j].ID
		}
		return missing[i].At.Before(missing[j].At)
	})
	if len(missing) > 0 {
		if err := s.appendAuditLocked(missing); err != nil {
			return fmt.Errorf("修复审计日志: %w", err)
		}
	}
	return nil
}

func (s *Store) readAuditLog() (map[string]domain.AuditEvent, error) {
	result := map[string]domain.AuditEvent{}
	file, err := os.Open(s.auditPath)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return nil, fmt.Errorf("打开审计日志: %w", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 2<<20)
	line := 0
	for scanner.Scan() {
		line++
		if strings.TrimSpace(scanner.Text()) == "" {
			continue
		}
		var event domain.AuditEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, fmt.Errorf("解析审计日志第 %d 行: %w", line, err)
		}
		if event.ID == "" || event.IncidentID == "" || event.Type == "" || event.At.IsZero() || event.Revision < 1 {
			return nil, fmt.Errorf("审计日志第 %d 行字段不完整", line)
		}
		if _, duplicate := result[event.ID]; duplicate {
			return nil, fmt.Errorf("审计日志第 %d 行事件 ID 重复: %s", line, event.ID)
		}
		result[event.ID] = event
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取审计日志: %w", err)
	}
	return result, nil
}
