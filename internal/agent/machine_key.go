package agent

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sort"

	"golang.org/x/crypto/hkdf"
)

// machineKeySalt is a fixed salt for key derivation.
var machineKeySalt = []byte("vcdeploy-agent-v1-machine-key")

// machineKeyInfo is the HKDF info string for the agent store key.
var machineKeyInfo = []byte("agent-store-encryption")

// deriveMachineKey derives a 32-byte encryption key from machine identifiers.
// The key is deterministic for the same machine, making it suitable for
// encrypting data that should only be readable on this machine.
func deriveMachineKey() ([]byte, error) {
	var sources [][]byte

	// Machine ID - primary identifier
	if machineID, err := getMachineID(); err == nil && len(machineID) > 0 {
		sources = append(sources, machineID)
	}

	// MAC address of first interface
	if mac, err := getPrimaryMAC(); err == nil && len(mac) > 0 {
		sources = append(sources, mac)
	}

	// Hostname
	if hostname, err := os.Hostname(); err == nil && hostname != "" {
		sources = append(sources, []byte(hostname))
	}

	// Need at least 2 sources for reasonable security
	if len(sources) < 2 {
		return nil, errors.New("insufficient machine identifiers")
	}

	// Combine sources with null separator
	combined := bytes.Join(sources, []byte{0})

	// Use HKDF to derive a 32-byte key
	reader := hkdf.New(sha256.New, combined, machineKeySalt, machineKeyInfo)
	key := make([]byte, 32)
	if _, err := io.ReadFull(reader, key); err != nil {
		return nil, err
	}

	return key, nil
}

// getMachineID reads the machine ID from the system.
func getMachineID() ([]byte, error) {
	switch runtime.GOOS {
	case "linux":
		// Linux: /etc/machine-id or /var/lib/dbus/machine-id
		paths := []string{
			"/etc/machine-id",
			"/var/lib/dbus/machine-id",
		}
		for _, path := range paths {
			if data, err := os.ReadFile(path); err == nil {
				return bytes.TrimSpace(data), nil
			}
		}
		return nil, errors.New("machine-id not found")

	case "darwin":
		// macOS: Use IOPlatformUUID from ioreg
		return getMacOSMachineID()

	default:
		return nil, errors.New("unsupported OS")
	}
}

// getMacOSMachineID retrieves the hardware UUID on macOS.
// It tries ioreg first, then falls back to sysctl.
func getMacOSMachineID() ([]byte, error) {
	// Primary: IOPlatformUUID via ioreg
	// #nosec G204 - hardcoded system command with literal arguments, no user input
	cmd := exec.Command("ioreg", "-rd1", "-c", "IOPlatformExpertDevice")
	output, err := cmd.Output()
	if err == nil {
		re := regexp.MustCompile(`"IOPlatformUUID"\s*=\s*"([^"]+)"`)
		if matches := re.FindSubmatch(output); len(matches) > 1 {
			return matches[1], nil
		}
	}

	// Fallback: sysctl hw.uuid
	// #nosec G204 - hardcoded system command with literal arguments, no user input
	cmd = exec.Command("sysctl", "-n", "hw.uuid")
	output, err = cmd.Output()
	if err == nil && len(output) > 0 {
		return bytes.TrimSpace(output), nil
	}

	return nil, errors.New("unable to get macOS machine ID: ioreg and sysctl failed")
}

// getPrimaryMAC returns the MAC address of the first non-loopback interface.
func getPrimaryMAC() ([]byte, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	// Sort for consistency
	sort.Slice(interfaces, func(i, j int) bool {
		return interfaces[i].Name < interfaces[j].Name
	})

	for _, iface := range interfaces {
		// Skip loopback and interfaces without MAC
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if len(iface.HardwareAddr) == 0 {
			continue
		}
		// Skip virtual interfaces (common patterns)
		if isVirtualInterface(iface.Name) {
			continue
		}
		return iface.HardwareAddr, nil
	}

	return nil, errors.New("no suitable network interface found")
}

// isVirtualInterface returns true if the interface name suggests it's virtual.
func isVirtualInterface(name string) bool {
	prefixes := []string{
		"veth", "docker", "br-", "virbr", "vnet",
		"lo", "tun", "tap",
	}
	for _, prefix := range prefixes {
		if len(name) >= len(prefix) && name[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

// makeDir creates a directory with the specified permissions.
func makeDir(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}
