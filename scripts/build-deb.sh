#!/bin/bash
# Build DEB package for VCDeploy
# Usage: ./scripts/build-deb.sh <version> <arch>
# Example: ./scripts/build-deb.sh 1.0.0 amd64

set -e

VERSION="${1:-$(git describe --tags --always | sed 's/^v//')}"
ARCH="${2:-amd64}"
PACKAGE_NAME="vcdeploy"
MAINTAINER="VCDeploy Team <vcdeploy@blackorder.dev>"
DESCRIPTION="VCDeploy - Deployment automation and orchestration tool"
HOMEPAGE="https://github.com/BlackOrder/vcdeploy"

# Check for required tools
if ! command -v fpm &> /dev/null; then
    echo "Error: fpm is required. Install with: gem install fpm"
    exit 1
fi

# Create working directory
WORKDIR=$(mktemp -d)
trap "rm -rf $WORKDIR" EXIT

# Create directory structure
mkdir -p "$WORKDIR/usr/local/bin"
mkdir -p "$WORKDIR/etc/vcdeploy"
mkdir -p "$WORKDIR/var/lib/vcdeploy"
mkdir -p "$WORKDIR/var/log/vcdeploy"
mkdir -p "$WORKDIR/lib/systemd/system"
mkdir -p "$WORKDIR/usr/share/doc/vcdeploy"

# Build binaries if they don't exist
if [ ! -f "vcdeploy" ]; then
    echo "Building vcdeploy..."
    GOARCH=$ARCH go build -ldflags="-s -w -X main.version=$VERSION" -o vcdeploy ./cmd/vcdeploy
fi

if [ ! -f "vcdeploy-agent" ]; then
    echo "Building vcdeploy-agent..."
    GOARCH=$ARCH go build -ldflags="-s -w -X main.version=$VERSION" -o vcdeploy-agent ./cmd/vcdeploy-agent
fi

# Copy binaries
cp vcdeploy "$WORKDIR/usr/local/bin/"
cp vcdeploy-agent "$WORKDIR/usr/local/bin/"
chmod +x "$WORKDIR/usr/local/bin/vcdeploy"
chmod +x "$WORKDIR/usr/local/bin/vcdeploy-agent"

# Copy documentation
cp README.md "$WORKDIR/usr/share/doc/vcdeploy/"
cp LICENSE "$WORKDIR/usr/share/doc/vcdeploy/" 2>/dev/null || echo "No LICENSE file found"

# Copy systemd service files from canonical location
cp init/systemd/vcdeploy-master.service "$WORKDIR/lib/systemd/system/"
cp init/systemd/vcdeploy-agent.service "$WORKDIR/lib/systemd/system/"

# Create sample configuration
cat > "$WORKDIR/etc/vcdeploy/master.yaml.sample" << 'EOF'
# VCDeploy Master Configuration
# Copy this file to master.yaml and customize

server:
  listen: ":9000"
  tls:
    enabled: false
    cert_file: ""
    key_file: ""

database:
  path: /var/lib/vcdeploy/vcdeploy.db

logging:
  level: info
  file: /var/log/vcdeploy/master.log

auth:
  session_ttl: 24h
  api_key_ttl: 720h  # 30 days
EOF

cat > "$WORKDIR/etc/vcdeploy/agent.yaml.sample" << 'EOF'
# VCDeploy Agent Configuration
# Copy this file to agent.yaml and customize

master:
  address: "localhost:9000"
  tls:
    enabled: false
    ca_cert: ""

agent:
  name: ""  # Auto-detected if empty
  labels:
    environment: production

logging:
  level: info
  file: /var/log/vcdeploy/agent.log
EOF

# Create post-install script
cat > "$WORKDIR/postinst" << 'EOF'
#!/bin/bash
set -e

# Create vcdeploy user if it doesn't exist
if ! id vcdeploy &> /dev/null; then
    useradd -r -s /bin/false -d /var/lib/vcdeploy -c "VCDeploy Service" vcdeploy
fi

# Set ownership
chown -R vcdeploy:vcdeploy /var/lib/vcdeploy
chown -R vcdeploy:vcdeploy /var/log/vcdeploy
chown -R root:vcdeploy /etc/vcdeploy
chmod 750 /etc/vcdeploy

# Reload systemd
systemctl daemon-reload

echo ""
echo "VCDeploy installed successfully!"
echo ""
echo "To get started:"
echo "  1. Copy sample configs: cp /etc/vcdeploy/*.yaml.sample /etc/vcdeploy/*.yaml"
echo "  2. Edit configuration files in /etc/vcdeploy/"
echo "  3. Start the service: systemctl start vcdeploy-master"
echo "  4. Enable on boot: systemctl enable vcdeploy-master"
echo ""
EOF
chmod +x "$WORKDIR/postinst"

# Create pre-remove script
cat > "$WORKDIR/prerm" << 'EOF'
#!/bin/bash
set -e

# Stop services before removal
systemctl stop vcdeploy-master 2>/dev/null || true
systemctl stop vcdeploy-agent 2>/dev/null || true
systemctl disable vcdeploy-master 2>/dev/null || true
systemctl disable vcdeploy-agent 2>/dev/null || true
EOF
chmod +x "$WORKDIR/prerm"

# Build DEB package with fpm
DEB_ARCH=$ARCH
if [ "$ARCH" = "amd64" ]; then
    DEB_ARCH="amd64"
elif [ "$ARCH" = "arm64" ]; then
    DEB_ARCH="arm64"
fi

fpm -s dir -t deb \
    -n "$PACKAGE_NAME" \
    -v "$VERSION" \
    -a "$DEB_ARCH" \
    --maintainer "$MAINTAINER" \
    --description "$DESCRIPTION" \
    --url "$HOMEPAGE" \
    --license "Apache-2.0" \
    --deb-priority "optional" \
    --category "admin" \
    --after-install "$WORKDIR/postinst" \
    --before-remove "$WORKDIR/prerm" \
    --deb-systemd "$WORKDIR/lib/systemd/system/vcdeploy-master.service" \
    --deb-systemd "$WORKDIR/lib/systemd/system/vcdeploy-agent.service" \
    --deb-no-default-config-files \
    --depends "ca-certificates" \
    --config-files /etc/vcdeploy/ \
    -C "$WORKDIR" \
    usr/local/bin/vcdeploy \
    usr/local/bin/vcdeploy-agent \
    etc/vcdeploy/ \
    usr/share/doc/vcdeploy/

echo "DEB package built: vcdeploy_${VERSION}_${DEB_ARCH}.deb"
