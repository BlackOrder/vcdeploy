#!/bin/bash
# Post-installation script for VCDeploy packages

set -e

# Create vcdeploy user if it doesn't exist
if ! id vcdeploy &> /dev/null 2>&1; then
    if command -v useradd &> /dev/null; then
        # Linux
        useradd -r -s /bin/false -d /var/lib/vcdeploy -c "VCDeploy Service" vcdeploy || true
    elif command -v dseditgroup &> /dev/null; then
        # macOS
        dscl . -create /Users/vcdeploy 2>/dev/null || true
    fi
fi

# Create and set permissions on directories
if [ -d /var/lib/vcdeploy ]; then
    chown -R vcdeploy:vcdeploy /var/lib/vcdeploy 2>/dev/null || true
fi

if [ -d /var/log/vcdeploy ]; then
    chown -R vcdeploy:vcdeploy /var/log/vcdeploy 2>/dev/null || true
fi

if [ -d /etc/vcdeploy ]; then
    chown -R root:vcdeploy /etc/vcdeploy 2>/dev/null || true
    chmod 750 /etc/vcdeploy 2>/dev/null || true
fi

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
echo "Documentation: https://github.com/BlackOrder/vcdeploy"
echo ""

exit 0
