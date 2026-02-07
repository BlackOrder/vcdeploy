#!/bin/bash
# VCDeploy Master Installation Script
# Supports: Ubuntu/Debian, CentOS/RHEL, Fedora

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

VERSION="${VCDEPLOY_VERSION:-latest}"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/vcdeploy"
DATA_DIR="/var/lib/vcdeploy"
LOG_DIR="/var/log/vcdeploy"

# Detect OS
detect_os() {
    if [ -f /etc/os-release ]; then
        . /etc/os-release
        OS=$ID
        VERSION_ID=$VERSION_ID
    elif [ -f /etc/redhat-release ]; then
        OS="rhel"
    else
        echo -e "${RED}Unsupported OS${NC}"
        exit 1
    fi
}

# Check if running as root
check_root() {
    if [ "$EUID" -ne 0 ]; then
        echo -e "${RED}Please run as root${NC}"
        exit 1
    fi
}

# Create vcdeploy user
create_user() {
    if ! id -u vcdeploy > /dev/null 2>&1; then
        echo -e "${YELLOW}Creating vcdeploy user...${NC}"
        useradd --system --shell /bin/false --home-dir $DATA_DIR vcdeploy
    fi
}

# Create directories
create_directories() {
    echo -e "${YELLOW}Creating directories...${NC}"
    mkdir -p $CONFIG_DIR
    mkdir -p $DATA_DIR/{backups,static,templates}
    mkdir -p $LOG_DIR
    
    chown -R vcdeploy:vcdeploy $DATA_DIR
    chown -R vcdeploy:vcdeploy $LOG_DIR
    chmod 750 $CONFIG_DIR
    chmod 750 $DATA_DIR
    chmod 750 $LOG_DIR
}

# Download and install binary
install_binary() {
    echo -e "${YELLOW}Downloading vcdeploy ${VERSION}...${NC}"
    
    ARCH=$(uname -m)
    case $ARCH in
        x86_64)
            ARCH="amd64"
            ;;
        aarch64)
            ARCH="arm64"
            ;;
        *)
            echo -e "${RED}Unsupported architecture: $ARCH${NC}"
            exit 1
            ;;
    esac
    
    if [ "$VERSION" = "latest" ]; then
        DOWNLOAD_URL="https://github.com/BlackOrder/vcdeploy/releases/latest/download/vcdeploy-linux-${ARCH}"
    else
        DOWNLOAD_URL="https://github.com/BlackOrder/vcdeploy/releases/download/${VERSION}/vcdeploy-linux-${ARCH}"
    fi
    
    curl -fsSL -o /tmp/vcdeploy "$DOWNLOAD_URL"
    chmod +x /tmp/vcdeploy
    mv /tmp/vcdeploy $INSTALL_DIR/vcdeploy
    
    echo -e "${GREEN}Installed vcdeploy to $INSTALL_DIR/vcdeploy${NC}"
}

# Create default config
create_config() {
    if [ ! -f $CONFIG_DIR/master.yaml ]; then
        echo -e "${YELLOW}Creating default configuration...${NC}"
        cat > $CONFIG_DIR/master.yaml << 'EOF'
# VCDeploy Master Configuration
server:
  listen: ":9000"
  tls:
    enabled: false
    cert: ""
    key: ""

grpc:
  listen: ":9001"

ssh:
  default_user: deploy
  default_key: /etc/vcdeploy/deploy_key
  connection_timeout: 30s
  keepalive_interval: 10s
  idle_timeout: 5m

security:
  key_rotation:
    enabled: true
    interval: 720h  # 30 days
  session_timeout: 24h
  require_2fa_admin: true

backup:
  database:
    enabled: true
    interval: 24h
    retention: 168h  # 7 days
    path: /var/lib/vcdeploy/backups
  config:
    versions: 5

logs:
  deployment:
    retention: 720h  # 30 days
    max_size_mb: 100
  audit:
    retention: 8760h  # 1 year
    export:
      enabled: false
  application:
    level: info
    retention: 168h  # 7 days
  rotation:
    schedule: "0 0 * * *"  # daily at midnight

webhooks:
  github:
    enabled: true
    path: /webhook/github
  gitlab:
    enabled: true
    path: /webhook/gitlab
  bitbucket:
    enabled: true
    path: /webhook/bitbucket

notifications:
  providers:
    slack:
      enabled: false
    email:
      enabled: false
    webhook:
      enabled: false

api:
  enabled: true

appearance:
  theme: dark
EOF
        chown vcdeploy:vcdeploy $CONFIG_DIR/master.yaml
        chmod 640 $CONFIG_DIR/master.yaml
    else
        echo -e "${YELLOW}Configuration file already exists, skipping...${NC}"
    fi
}

# Install systemd service
install_service() {
    echo -e "${YELLOW}Installing systemd service...${NC}"
    
    cat > /etc/systemd/system/vcdeploy-master.service << 'EOF'
[Unit]
Description=VCDeploy Master Server
Documentation=https://github.com/BlackOrder/vcdeploy
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=vcdeploy
Group=vcdeploy
ExecStart=/usr/local/bin/vcdeploy master start --config /etc/vcdeploy/master.yaml
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
RestartSec=5
TimeoutStartSec=30
TimeoutStopSec=30
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=/var/lib/vcdeploy /var/log/vcdeploy
LimitNOFILE=65536
LimitNPROC=4096

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    echo -e "${GREEN}Systemd service installed${NC}"
}

# Generate SSH key for deployments
generate_ssh_key() {
    if [ ! -f $CONFIG_DIR/deploy_key ]; then
        echo -e "${YELLOW}Generating deployment SSH key...${NC}"
        ssh-keygen -t ed25519 -f $CONFIG_DIR/deploy_key -N "" -C "vcdeploy@$(hostname)"
        chown vcdeploy:vcdeploy $CONFIG_DIR/deploy_key $CONFIG_DIR/deploy_key.pub
        chmod 600 $CONFIG_DIR/deploy_key
        chmod 644 $CONFIG_DIR/deploy_key.pub
        echo -e "${GREEN}SSH key generated${NC}"
        echo -e "${YELLOW}Public key (add to deployment targets):${NC}"
        cat $CONFIG_DIR/deploy_key.pub
    fi
}

# Initialize database and create admin user
initialize() {
    echo -e "${YELLOW}Initializing database...${NC}"
    # The database will be created on first run
    touch $DATA_DIR/vcdeploy.db
    chown vcdeploy:vcdeploy $DATA_DIR/vcdeploy.db
    
    echo -e "${GREEN}Database initialized${NC}"
}

# Print completion message
complete() {
    echo ""
    echo -e "${GREEN}╔══════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║           VCDeploy Master Installation Complete!             ║${NC}"
    echo -e "${GREEN}╚══════════════════════════════════════════════════════════════╝${NC}"
    echo ""
    echo "To start the service:"
    echo "  sudo systemctl start vcdeploy-master"
    echo "  sudo systemctl enable vcdeploy-master"
    echo ""
    echo "Configuration file: $CONFIG_DIR/master.yaml"
    echo "Web UI will be available at: http://$(hostname -I | awk '{print $1}'):9000"
    echo ""
    echo "For first-time setup:"
    echo "  1. Edit $CONFIG_DIR/master.yaml to configure TLS, notifications, etc."
    echo "  2. Create an admin user: vcdeploy admin create-user --username admin"
    echo "  3. Start the service: systemctl start vcdeploy-master"
    echo ""
}

# Main
main() {
    echo -e "${GREEN}╔══════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║                VCDeploy Master Installer                     ║${NC}"
    echo -e "${GREEN}╚══════════════════════════════════════════════════════════════╝${NC}"
    echo ""
    
    check_root
    detect_os
    create_user
    create_directories
    install_binary
    create_config
    install_service
    generate_ssh_key
    initialize
    complete
}

main "$@"
