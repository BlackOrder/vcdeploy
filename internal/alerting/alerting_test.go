package alerting

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/BlackOrder/vcdeploy/internal/storage"
	"go.uber.org/zap"
)

func TestDefaultThresholds(t *testing.T) {
	th := DefaultThresholds()

	if th.DiskWarningPercent != 80 {
		t.Errorf("DiskWarningPercent = %v, want 80", th.DiskWarningPercent)
	}
	if th.DiskCriticalPercent != 90 {
		t.Errorf("DiskCriticalPercent = %v, want 90", th.DiskCriticalPercent)
	}
	if th.MemoryWarningPercent != 85 {
		t.Errorf("MemoryWarningPercent = %v, want 85", th.MemoryWarningPercent)
	}
	if th.CPUWarningPercent != 90 {
		t.Errorf("CPUWarningPercent = %v, want 90", th.CPUWarningPercent)
	}
	if th.DeploymentTimeout != 30*time.Minute {
		t.Errorf("DeploymentTimeout = %v, want 30m", th.DeploymentTimeout)
	}
	if th.AlertCooldown != 15*time.Minute {
		t.Errorf("AlertCooldown = %v, want 15m", th.AlertCooldown)
	}
}

func TestNewManager(t *testing.T) {
	logger := zap.NewNop()
	th := DefaultThresholds()

	// Test with nil logger
	m := NewManager(nil, nil, th)
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	if m.cooldowns == nil {
		t.Error("cooldowns map is nil")
	}

	// Test with logger
	m = NewManager(nil, logger, th)
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	if m.logger == nil {
		t.Error("logger is nil")
	}
}

func TestManager_canAlert(t *testing.T) {
	th := Thresholds{
		AlertCooldown: 100 * time.Millisecond,
	}
	m := NewManager(nil, nil, th)

	key := "test_alert"

	// First call should return true
	if !m.canAlert(key) {
		t.Error("first canAlert should return true")
	}

	// Immediate second call should return false (within cooldown)
	if m.canAlert(key) {
		t.Error("second canAlert within cooldown should return false")
	}

	// After cooldown, should return true again
	time.Sleep(150 * time.Millisecond)
	if !m.canAlert(key) {
		t.Error("canAlert after cooldown should return true")
	}
}

func TestManager_CheckAgentDisconnect(t *testing.T) {
	th := Thresholds{
		AlertCooldown: 100 * time.Millisecond,
	}
	m := NewManager(nil, zap.NewNop(), th)

	ctx := context.Background()
	agent := &storage.Agent{
		ID:         "agent-1",
		Hostname:   "test-host",
		LastSeenAt: time.Now().Add(-5 * time.Minute),
	}

	// First disconnect should not cause panic (even with nil notifier)
	m.CheckAgentDisconnect(ctx, agent)

	// Second disconnect within cooldown should be suppressed
	// (can be verified by checking cooldown was set)
	key := "agent_disconnect_agent-1"
	m.mu.RLock()
	_, exists := m.cooldowns[key]
	m.mu.RUnlock()
	if !exists {
		t.Error("cooldown should be set after disconnect alert")
	}
}

func TestManager_CheckAgentReconnect(t *testing.T) {
	th := Thresholds{
		AlertCooldown: 100 * time.Millisecond,
	}
	m := NewManager(nil, zap.NewNop(), th)

	ctx := context.Background()
	agent := &storage.Agent{
		ID:       "agent-1",
		Hostname: "test-host",
	}

	// Set a disconnect cooldown first
	disconnectKey := "agent_disconnect_agent-1"
	m.mu.Lock()
	m.cooldowns[disconnectKey] = time.Now()
	m.mu.Unlock()

	// Reconnect should clear the disconnect cooldown
	m.CheckAgentReconnect(ctx, agent)

	m.mu.RLock()
	_, exists := m.cooldowns[disconnectKey]
	m.mu.RUnlock()
	if exists {
		t.Error("disconnect cooldown should be cleared on reconnect")
	}
}

func TestManager_CheckDiskUsage(t *testing.T) {
	th := Thresholds{
		DiskWarningPercent:  80,
		DiskCriticalPercent: 90,
		AlertCooldown:       100 * time.Millisecond,
	}
	m := NewManager(nil, zap.NewNop(), th)

	ctx := context.Background()

	tests := []struct {
		name        string
		diskPercent float64
		expectAlert bool
		alertKey    string
	}{
		{"below warning", 50, false, ""},
		{"at warning", 80, true, "disk_warning_agent-1"},
		{"at critical", 90, true, "disk_critical_agent-1"},
		{"above critical", 95, true, "disk_critical_agent-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m.ClearAllCooldowns()
			m.CheckDiskUsage(ctx, "agent-1", "host-1", tt.diskPercent)

			if tt.expectAlert && tt.alertKey != "" {
				m.mu.RLock()
				_, exists := m.cooldowns[tt.alertKey]
				m.mu.RUnlock()
				if !exists {
					t.Errorf("expected cooldown for %s to be set", tt.alertKey)
				}
			}
		})
	}
}

func TestManager_CheckMemoryUsage(t *testing.T) {
	th := Thresholds{
		MemoryWarningPercent: 85,
		AlertCooldown:        100 * time.Millisecond,
	}
	m := NewManager(nil, zap.NewNop(), th)

	ctx := context.Background()

	// Below threshold - no alert
	m.CheckMemoryUsage(ctx, "agent-1", "host-1", 50)
	m.mu.RLock()
	_, exists := m.cooldowns["memory_warning_agent-1"]
	m.mu.RUnlock()
	if exists {
		t.Error("should not set cooldown for below-threshold memory")
	}

	// Above threshold - alert
	m.CheckMemoryUsage(ctx, "agent-1", "host-1", 90)
	m.mu.RLock()
	_, exists = m.cooldowns["memory_warning_agent-1"]
	m.mu.RUnlock()
	if !exists {
		t.Error("should set cooldown for above-threshold memory")
	}
}

func TestManager_CheckCPUUsage(t *testing.T) {
	th := Thresholds{
		CPUWarningPercent: 90,
		AlertCooldown:     100 * time.Millisecond,
	}
	m := NewManager(nil, zap.NewNop(), th)

	ctx := context.Background()

	// Below threshold - no alert
	m.CheckCPUUsage(ctx, "agent-1", "host-1", 50)
	m.mu.RLock()
	_, exists := m.cooldowns["cpu_warning_agent-1"]
	m.mu.RUnlock()
	if exists {
		t.Error("should not set cooldown for below-threshold CPU")
	}

	// At threshold - alert
	m.CheckCPUUsage(ctx, "agent-1", "host-1", 90)
	m.mu.RLock()
	_, exists = m.cooldowns["cpu_warning_agent-1"]
	m.mu.RUnlock()
	if !exists {
		t.Error("should set cooldown for above-threshold CPU")
	}
}

func TestManager_CheckDeploymentStuck(t *testing.T) {
	th := Thresholds{
		DeploymentTimeout: 100 * time.Millisecond,
		AlertCooldown:     100 * time.Millisecond,
	}
	m := NewManager(nil, zap.NewNop(), th)

	ctx := context.Background()

	// Recent deployment - no alert
	m.CheckDeploymentStuck(ctx, "deploy-1", "project-1", time.Now())
	m.mu.RLock()
	_, exists := m.cooldowns["deployment_stuck_deploy-1"]
	m.mu.RUnlock()
	if exists {
		t.Error("should not alert for recent deployment")
	}

	// Old deployment - alert
	m.CheckDeploymentStuck(ctx, "deploy-1", "project-1", time.Now().Add(-1*time.Hour))
	m.mu.RLock()
	_, exists = m.cooldowns["deployment_stuck_deploy-1"]
	m.mu.RUnlock()
	if !exists {
		t.Error("should alert for stuck deployment")
	}
}

func TestManager_CheckAgentMetrics(t *testing.T) {
	th := Thresholds{
		DiskWarningPercent:   80,
		DiskCriticalPercent:  90,
		MemoryWarningPercent: 85,
		CPUWarningPercent:    90,
		AlertCooldown:        100 * time.Millisecond,
	}
	m := NewManager(nil, zap.NewNop(), th)

	ctx := context.Background()

	// All high metrics - should trigger all alerts
	m.CheckAgentMetrics(ctx, "agent-1", "host-1", 95, 90, 92)

	m.mu.RLock()
	_, cpuExists := m.cooldowns["cpu_warning_agent-1"]
	_, memExists := m.cooldowns["memory_warning_agent-1"]
	_, diskExists := m.cooldowns["disk_critical_agent-1"]
	m.mu.RUnlock()

	if !cpuExists {
		t.Error("should trigger CPU alert")
	}
	if !memExists {
		t.Error("should trigger memory alert")
	}
	if !diskExists {
		t.Error("should trigger disk critical alert")
	}
}

func TestManager_ClearCooldown(t *testing.T) {
	th := DefaultThresholds()
	m := NewManager(nil, nil, th)

	// Set a cooldown
	m.mu.Lock()
	m.cooldowns["test_key"] = time.Now()
	m.mu.Unlock()

	// Clear it
	m.ClearCooldown("test_key")

	m.mu.RLock()
	_, exists := m.cooldowns["test_key"]
	m.mu.RUnlock()
	if exists {
		t.Error("cooldown should be cleared")
	}
}

func TestManager_ClearAllCooldowns(t *testing.T) {
	th := DefaultThresholds()
	m := NewManager(nil, nil, th)

	// Set multiple cooldowns
	m.mu.Lock()
	m.cooldowns["key1"] = time.Now()
	m.cooldowns["key2"] = time.Now()
	m.cooldowns["key3"] = time.Now()
	m.mu.Unlock()

	// Clear all
	m.ClearAllCooldowns()

	m.mu.RLock()
	count := len(m.cooldowns)
	m.mu.RUnlock()
	if count != 0 {
		t.Errorf("expected 0 cooldowns, got %d", count)
	}
}

func TestManager_ConcurrentAccess(t *testing.T) {
	th := Thresholds{
		DiskWarningPercent:   80,
		DiskCriticalPercent:  90,
		MemoryWarningPercent: 85,
		CPUWarningPercent:    90,
		AlertCooldown:        1 * time.Millisecond,
	}
	m := NewManager(nil, zap.NewNop(), th)

	ctx := context.Background()
	var wg sync.WaitGroup

	// Run concurrent checks
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			m.CheckCPUUsage(ctx, "agent-1", "host-1", 95)
			m.CheckMemoryUsage(ctx, "agent-1", "host-1", 90)
			m.CheckDiskUsage(ctx, "agent-1", "host-1", 85)
			m.canAlert("concurrent_test")
			m.ClearCooldown("concurrent_test")
		}(i)
	}

	wg.Wait()
	// If we reach here without deadlock or race, test passes
}

func TestAlertTypeConstants(t *testing.T) {
	// Verify alert type constants are defined correctly
	tests := []struct {
		alertType AlertType
		expected  string
	}{
		{AlertAgentDisconnected, "agent_disconnected"},
		{AlertAgentReconnected, "agent_reconnected"},
		{AlertDiskWarning, "disk_warning"},
		{AlertDiskCritical, "disk_critical"},
		{AlertHighMemory, "high_memory"},
		{AlertHighCPU, "high_cpu"},
		{AlertDeploymentStuck, "deployment_stuck"},
		{AlertHighErrorRate, "high_error_rate"},
	}

	for _, tt := range tests {
		if string(tt.alertType) != tt.expected {
			t.Errorf("AlertType %v = %s, want %s", tt.alertType, tt.alertType, tt.expected)
		}
	}
}
