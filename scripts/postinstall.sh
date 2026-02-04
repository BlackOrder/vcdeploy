#!/bin/bash
# Post-installation script for VCDeploy packages

set -e

# Create vcdeploy group if it doesn't exist
if ! getent group vcdeploy &> /dev/null 2>&1; then
    if command -v groupadd &> /dev/null; then
        # Linux
        groupadd -r vcdeploy || true
    elif command -v dseditgroup &> /dev/null; then
        # macOS
        dseditgroup -o create -r "VCDeploy Service" vcdeploy 2>/dev/null || true
    fi
fi

# Create vcdeploy user if it doesn't exist
if ! id vcdeploy &> /dev/null 2>&1; then
    if command -v useradd &> /dev/null; then
        # Linux
        useradd -r -s /bin/false -d /var/lib/vcdeploy -g vcdeploy -c "VCDeploy Service" vcdeploy || true
    elif command -v dseditgroup &> /dev/null; then
        # macOS
        dscl . -create /Users/vcdeploy 2>/dev/null || true
    fi
fi

# Create socket directory for Unix socket (local CLI access)
if [ ! -d /var/run/vcdeploy ]; then
    mkdir -p /var/run/vcdeploy
fi
chown root:vcdeploy /var/run/vcdeploy 2>/dev/null || true
chmod 0750 /var/run/vcdeploy 2>/dev/null || true

# Create and set permissions on data directories
if [ ! -d /var/lib/vcdeploy ]; then
    mkdir -p /var/lib/vcdeploy
fi
chown -R vcdeploy:vcdeploy /var/lib/vcdeploy 2>/dev/null || true
chmod 0750 /var/lib/vcdeploy 2>/dev/null || true

if [ ! -d /var/log/vcdeploy ]; then
    mkdir -p /var/log/vcdeploy
fi
chown -R vcdeploy:vcdeploy /var/log/vcdeploy 2>/dev/null || true

if [ ! -d /etc/vcdeploy ]; then
    mkdir -p /etc/vcdeploy
fi
chown -R root:vcdeploy /etc/vcdeploy 2>/dev/null || true
chmod 750 /etc/vcdeploy 2>/dev/null || true

# Reload systemd if available
if command -v systemctl &> /dev/null; then
    systemctl daemon-reload 2>/dev/null || true
fi

echo ""
echo "============================================"
echo "VCDeploy installed successfully!"
echo "============================================"
echo ""
echo "To get started:"
echo "  1. Initialize: vcdeploy init"
echo "  2. Start: vcdeploy master start"
echo ""
echo "Or use systemd:"
echo "  sudo systemctl enable vcdeploy-master"
echo "  sudo systemctl start vcdeploy-master"
echo ""
echo "To allow a user to run CLI commands without authentication:"
echo "  sudo usermod -aG vcdeploy USERNAME"
echo ""
echo "Documentation: https://github.com/BlackOrder/vcdeploy"
echo ""

exit 0
