#!/bin/bash
# VCDeploy Agent Installation Script
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

# Create directories
create_directories() {
    echo -e "${YELLOW}Creating directories...${NC}"
    mkdir -p $CONFIG_DIR
    chmod 750 $CONFIG_DIR
}

# Download and install binary
install_binary() {
    echo -e "${YELLOW}Downloading vcdeploy-agent ${VERSION}...${NC}"
    
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
        DOWNLOAD_URL="https://github.com/BlackOrder/vcdeploy/releases/latest/download/vcdeploy-agent-linux-${ARCH}"
    else
        DOWNLOAD_URL="https://github.com/BlackOrder/vcdeploy/releases/download/${VERSION}/vcdeploy-agent-linux-${ARCH}"
    fi
    
    curl -fsSL -o /tmp/vcdeploy-agent "$DOWNLOAD_URL"
    chmod +x /tmp/vcdeploy-agent
    mv /tmp/vcdeploy-agent $INSTALL_DIR/vcdeploy-agent
    
    echo -e "${GREEN}Installed vcdeploy-agent to $INSTALL_DIR/vcdeploy-agent${NC}"
}

# Create default config
create_config() {
    if [ ! -f $CONFIG_DIR/agent.yaml ]; then
        echo -e "${YELLOW}Creating default configuration...${NC}"
        cat > $CONFIG_DIR/agent.yaml << 'EOF'
# VCDeploy Agent Configuration
# Edit this file after running: vcdeploy-agent register --master <master-address>

agent:
  id: ""  # Will be auto-generated during registration
  tags: []
  
master:
  address: ""  # Set during registration
  cert: /etc/vcdeploy/master.crt  # Master's TLS certificate
  reconnect:
    heartbeat_interval: 30s
    initial_delay: 1s
    max_delay: 60s

logging:
  level: info
  file: /var/log/vcdeploy-agent.log

deploy:
  work_dir: /var/lib/vcdeploy-agent
  timeout: 30m
  
# Optional: SSH settings for local deployments
ssh:
  allowed_users:
    - deploy
    - www-data
EOF
        chmod 640 $CONFIG_DIR/agent.yaml
    else
        echo -e "${YELLOW}Configuration file already exists, skipping...${NC}"
    fi
}

# Install systemd service
install_service() {
    echo -e "${YELLOW}Installing systemd service...${NC}"
    
    cat > /etc/systemd/system/vcdeploy-agent.service << 'EOF'
[Unit]
Description=VCDeploy Agent
Documentation=https://github.com/BlackOrder/vcdeploy
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/vcdeploy-agent run --config /etc/vcdeploy/agent.yaml
ExecReload=/bin/kill -HUP $MAINPID
Restart=always
RestartSec=5
TimeoutStartSec=30
TimeoutStopSec=30
LimitNOFILE=65536
LimitNPROC=4096

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    echo -e "${GREEN}Systemd service installed${NC}"
}

# Print completion message
complete() {
    echo ""
    echo -e "${GREEN}╔══════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║           VCDeploy Agent Installation Complete!              ║${NC}"
    echo -e "${GREEN}╚══════════════════════════════════════════════════════════════╝${NC}"
    echo ""
    echo "Next steps:"
    echo ""
    echo "1. Register with the master server:"
    echo "   vcdeploy-agent register --master <master-address> --token <token>"
    echo ""
    echo "2. Start the service:"
    echo "   sudo systemctl start vcdeploy-agent"
    echo "   sudo systemctl enable vcdeploy-agent"
    echo ""
    echo "Configuration file: $CONFIG_DIR/agent.yaml"
    echo ""
}

# Register command
register() {
    MASTER_ADDR=""
    TOKEN=""
    
    while [[ $# -gt 0 ]]; do
        case $1 in
            --master)
                MASTER_ADDR="$2"
                shift 2
                ;;
            --token)
                TOKEN="$2"
                shift 2
                ;;
            *)
                shift
                ;;
        esac
    done
    
    if [ -z "$MASTER_ADDR" ] || [ -z "$TOKEN" ]; then
        echo -e "${RED}Usage: $0 register --master <address> --token <token>${NC}"
        exit 1
    fi
    
    echo -e "${YELLOW}Registering agent with master at $MASTER_ADDR...${NC}"
    
    # Generate agent ID
    AGENT_ID=$(cat /proc/sys/kernel/random/uuid)
    
    # Update config with master address
    sed -i "s|address: \"\"|address: \"$MASTER_ADDR\"|" $CONFIG_DIR/agent.yaml
    sed -i "s|id: \"\"|id: \"$AGENT_ID\"|" $CONFIG_DIR/agent.yaml
    
    # Download master certificate
    curl -fsSL "http://${MASTER_ADDR}/api/v1/ca-cert" -o $CONFIG_DIR/master.crt 2>/dev/null || {
        echo -e "${YELLOW}Could not download master certificate, TLS might not be enabled${NC}"
    }
    
    echo -e "${GREEN}Agent registered successfully!${NC}"
    echo "Agent ID: $AGENT_ID"
    echo ""
    echo "Start the agent with:"
    echo "  sudo systemctl start vcdeploy-agent"
}

# Main
main() {
    if [ "$1" = "register" ]; then
        shift
        register "$@"
        exit 0
    fi
    
    echo -e "${GREEN}╔══════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║                VCDeploy Agent Installer                      ║${NC}"
    echo -e "${GREEN}╚══════════════════════════════════════════════════════════════╝${NC}"
    echo ""
    
    check_root
    detect_os
    create_directories
    install_binary
    create_config
    install_service
    complete
}

main "$@"
