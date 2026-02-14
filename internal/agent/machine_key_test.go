package agent

import (
	"runtime"
	"testing"
)

func TestDeriveMachineKey(t *testing.T) {
	// Test that machine key derivation works and is consistent
	key1, err := deriveMachineKey()
	if err != nil {
		t.Fatalf("deriveMachineKey: %v", err)
	}

	if len(key1) != 32 {
		t.Errorf("key length: got %d, want 32", len(key1))
	}

	// Key should be consistent across calls
	key2, err := deriveMachineKey()
	if err != nil {
		t.Fatalf("deriveMachineKey (second call): %v", err)
	}

	if string(key1) != string(key2) {
		t.Error("machine key is not consistent across calls")
	}
}

func TestDeriveMachineKey_Consistent(t *testing.T) {
	// Call multiple times to ensure consistency
	var keys [][]byte
	for i := 0; i < 5; i++ {
		key, err := deriveMachineKey()
		if err != nil {
			t.Fatalf("deriveMachineKey call %d: %v", i, err)
		}
		keys = append(keys, key)
	}

	for i := 1; i < len(keys); i++ {
		if string(keys[i]) != string(keys[0]) {
			t.Errorf("key %d differs from key 0", i)
		}
	}
}

func TestGetMachineID(t *testing.T) {
	// This test may not work in all environments (e.g., containers)
	machineID, err := getMachineID()
	if err != nil {
		t.Skipf("getMachineID not available: %v", err)
	}

	if len(machineID) == 0 {
		t.Error("machine ID is empty")
	}

	t.Logf("Machine ID: %s", machineID)
}

func TestGetMachineID_Linux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-specific test")
	}

	machineID, err := getMachineID()
	if err != nil {
		t.Fatalf("getMachineID on Linux: %v", err)
	}

	// Linux machine-id is typically a 32-char hex string
	if len(machineID) < 16 {
		t.Errorf("Linux machine ID too short: %d bytes", len(machineID))
	}

	t.Logf("Linux machine ID: %s", machineID)
}

func TestGetMachineID_Darwin(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS-specific test")
	}

	machineID, err := getMachineID()
	if err != nil {
		t.Fatalf("getMachineID on macOS: %v", err)
	}

	// macOS UUID is typically in the format: XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX
	if len(machineID) < 32 {
		t.Errorf("macOS machine ID too short: %d bytes", len(machineID))
	}

	t.Logf("macOS machine ID: %s", machineID)
}

func TestGetMacOSMachineID_ParsesIoreg(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS-specific test")
	}

	// getMacOSMachineID should return a valid UUID
	machineID, err := getMacOSMachineID()
	if err != nil {
		t.Fatalf("getMacOSMachineID: %v", err)
	}

	// Should be a UUID format or similar identifier
	if len(machineID) < 32 {
		t.Errorf("macOS machine ID too short: got %d bytes, want >= 32", len(machineID))
	}

	t.Logf("macOS IOPlatformUUID: %s", machineID)
}

func TestGetPrimaryMAC(t *testing.T) {
	mac, err := getPrimaryMAC()
	if err != nil {
		t.Skipf("getPrimaryMAC not available: %v", err)
	}

	if len(mac) == 0 {
		t.Error("MAC address is empty")
	}

	// MAC addresses are typically 6 bytes
	if len(mac) != 6 {
		t.Logf("Warning: MAC address length is %d (expected 6)", len(mac))
	}

	t.Logf("Primary MAC: %x", mac)
}

func TestIsVirtualInterface(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"eth0", false},
		{"ens33", false},
		{"enp0s3", false},
		{"wlan0", false},
		{"docker0", true},
		{"br-abc123", true},
		{"veth1234", true},
		{"virbr0", true},
		{"lo", true},
		{"tun0", true},
		{"tap0", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isVirtualInterface(tt.name)
			if result != tt.expected {
				t.Errorf("isVirtualInterface(%q) = %v, want %v", tt.name, result, tt.expected)
			}
		})
	}
}
