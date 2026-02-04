package storage

import "testing"

func TestDeploymentStatus_String(t *testing.T) {
	tests := []struct {
		status DeploymentStatus
		want   string
	}{
		{DeploymentStatusPending, "pending"},
		{DeploymentStatusScheduled, "scheduled"},
		{DeploymentStatusRunning, "running"},
		{DeploymentStatusSuccess, "success"},
		{DeploymentStatusFailed, "failed"},
		{DeploymentStatusCancelled, "cancelled"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.status.String(); got != tt.want {
				t.Errorf("DeploymentStatus.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDeploymentStatus_IsTerminal(t *testing.T) {
	tests := []struct {
		status     DeploymentStatus
		isTerminal bool
	}{
		{DeploymentStatusPending, false},
		{DeploymentStatusScheduled, false},
		{DeploymentStatusRunning, false},
		{DeploymentStatusSuccess, true},
		{DeploymentStatusFailed, true},
		{DeploymentStatusCancelled, true},
	}

	for _, tt := range tests {
		t.Run(tt.status.String(), func(t *testing.T) {
			if got := tt.status.IsTerminal(); got != tt.isTerminal {
				t.Errorf("DeploymentStatus.IsTerminal() = %v, want %v", got, tt.isTerminal)
			}
		})
	}
}

func TestAgentStatus_String(t *testing.T) {
	tests := []struct {
		status AgentStatus
		want   string
	}{
		{AgentStatusOnline, "online"},
		{AgentStatusOffline, "offline"},
		{AgentStatusConnected, "connected"},
		{AgentStatusDisconnected, "disconnected"},
		{AgentStatusStale, "stale"},
		{AgentStatusPending, "pending"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.status.String(); got != tt.want {
				t.Errorf("AgentStatus.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAgentStatus_IsHealthy(t *testing.T) {
	tests := []struct {
		status    AgentStatus
		isHealthy bool
	}{
		{AgentStatusOnline, true},
		{AgentStatusOffline, false},
		{AgentStatusConnected, true},
		{AgentStatusDisconnected, false},
		{AgentStatusStale, false},
		{AgentStatusPending, false},
	}

	for _, tt := range tests {
		t.Run(tt.status.String(), func(t *testing.T) {
			if got := tt.status.IsHealthy(); got != tt.isHealthy {
				t.Errorf("AgentStatus.IsHealthy() = %v, want %v", got, tt.isHealthy)
			}
		})
	}
}

func TestProvisionStatus_String(t *testing.T) {
	tests := []struct {
		status ProvisionStatus
		want   string
	}{
		{ProvisionStatusPending, "pending"},
		{ProvisionStatusInProgress, "in_progress"},
		{ProvisionStatusCompleted, "completed"},
		{ProvisionStatusFailed, "failed"},
		{ProvisionStatusCancelled, "cancelled"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.status.String(); got != tt.want {
				t.Errorf("ProvisionStatus.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProvisionStatus_IsTerminal(t *testing.T) {
	tests := []struct {
		status     ProvisionStatus
		isTerminal bool
	}{
		{ProvisionStatusPending, false},
		{ProvisionStatusInProgress, false},
		{ProvisionStatusCompleted, true},
		{ProvisionStatusFailed, true},
		{ProvisionStatusCancelled, true},
	}

	for _, tt := range tests {
		t.Run(tt.status.String(), func(t *testing.T) {
			if got := tt.status.IsTerminal(); got != tt.isTerminal {
				t.Errorf("ProvisionStatus.IsTerminal() = %v, want %v", got, tt.isTerminal)
			}
		})
	}
}

func TestUpdateStatus_String(t *testing.T) {
	tests := []struct {
		status UpdateStatus
		want   string
	}{
		{UpdateStatusPending, "pending"},
		{UpdateStatusInProgress, "in_progress"},
		{UpdateStatusCompleted, "completed"},
		{UpdateStatusFailed, "failed"},
		{UpdateStatusRolledBack, "rolled_back"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.status.String(); got != tt.want {
				t.Errorf("UpdateStatus.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUpdateStatus_IsTerminal(t *testing.T) {
	tests := []struct {
		status     UpdateStatus
		isTerminal bool
	}{
		{UpdateStatusPending, false},
		{UpdateStatusInProgress, false},
		{UpdateStatusCompleted, true},
		{UpdateStatusFailed, true},
		{UpdateStatusRolledBack, true},
	}

	for _, tt := range tests {
		t.Run(tt.status.String(), func(t *testing.T) {
			if got := tt.status.IsTerminal(); got != tt.isTerminal {
				t.Errorf("UpdateStatus.IsTerminal() = %v, want %v", got, tt.isTerminal)
			}
		})
	}
}

func TestRollbackStatus_String(t *testing.T) {
	tests := []struct {
		status RollbackStatus
		want   string
	}{
		{RollbackStatusPending, "pending"},
		{RollbackStatusInProgress, "in_progress"},
		{RollbackStatusCompleted, "completed"},
		{RollbackStatusFailed, "failed"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.status.String(); got != tt.want {
				t.Errorf("RollbackStatus.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRollbackStatus_IsTerminal(t *testing.T) {
	tests := []struct {
		status     RollbackStatus
		isTerminal bool
	}{
		{RollbackStatusPending, false},
		{RollbackStatusInProgress, false},
		{RollbackStatusCompleted, true},
		{RollbackStatusFailed, true},
	}

	for _, tt := range tests {
		t.Run(tt.status.String(), func(t *testing.T) {
			if got := tt.status.IsTerminal(); got != tt.isTerminal {
				t.Errorf("RollbackStatus.IsTerminal() = %v, want %v", got, tt.isTerminal)
			}
		})
	}
}

func TestScheduledDeploymentStatus_String(t *testing.T) {
	tests := []struct {
		status ScheduledDeploymentStatus
		want   string
	}{
		{ScheduledDeploymentStatusPending, "pending"},
		{ScheduledDeploymentStatusTriggered, "triggered"},
		{ScheduledDeploymentStatusCancelled, "cancelled"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.status.String(); got != tt.want {
				t.Errorf("ScheduledDeploymentStatus.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestStatusTypeConversions verifies that string-to-status type conversions work correctly.
func TestStatusTypeConversions(t *testing.T) {
	t.Run("DeploymentStatus from string", func(t *testing.T) {
		status := DeploymentStatus("pending")
		if status != DeploymentStatusPending {
			t.Errorf("Expected DeploymentStatusPending, got %v", status)
		}
	})

	t.Run("AgentStatus from string", func(t *testing.T) {
		status := AgentStatus("online")
		if status != AgentStatusOnline {
			t.Errorf("Expected AgentStatusOnline, got %v", status)
		}
	})

	t.Run("ProvisionStatus from string", func(t *testing.T) {
		status := ProvisionStatus("completed")
		if status != ProvisionStatusCompleted {
			t.Errorf("Expected ProvisionStatusCompleted, got %v", status)
		}
	})

	t.Run("UpdateStatus from string", func(t *testing.T) {
		status := UpdateStatus("rolled_back")
		if status != UpdateStatusRolledBack {
			t.Errorf("Expected UpdateStatusRolledBack, got %v", status)
		}
	})

	t.Run("British spelling cancelled", func(t *testing.T) {
		// Verify we use British spelling
		if DeploymentStatusCancelled.String() != "cancelled" {
			t.Errorf("Expected 'cancelled' (British spelling), got %v", DeploymentStatusCancelled.String())
		}
		if ProvisionStatusCancelled.String() != "cancelled" {
			t.Errorf("Expected 'cancelled' (British spelling), got %v", ProvisionStatusCancelled.String())
		}
		if ScheduledDeploymentStatusCancelled.String() != "cancelled" {
			t.Errorf("Expected 'cancelled' (British spelling), got %v", ScheduledDeploymentStatusCancelled.String())
		}
	})
}
