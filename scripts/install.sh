#!/bin/bash
# VCDeploy Installation Script
# Supports: Linux (DEB, RPM, tarball), macOS (Homebrew, tarball)
# Usage: curl -fsSL https://raw.githubusercontent.com/BlackOrder/vcdeploy/main/scripts/install.sh | bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
REPO="BlackOrder/vcdeploy"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/vcdeploy"
DATA_DIR="/var/lib/vcdeploy"
LOG_DIR="/var/log/vcdeploy"

# Detect OS and architecture
detect_platform() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)
    
    case "$ARCH" in
        x86_64|amd64)
            ARCH="amd64"
            ;;
        aarch64|arm64)
            ARCH="arm64"
            ;;
        armv7l|armv6l)
            ARCH="arm"
            ;;
        *)
            echo -e "${RED}Unsupported architecture: $ARCH${NC}"
            exit 1
            ;;
    esac
    
    case "$OS" in
        linux)
            # Detect Linux distribution
            if [ -f /etc/os-release ]; then
                . /etc/os-release
                DISTRO=$ID
            elif [ -f /etc/redhat-release ]; then
                DISTRO="rhel"
            elif [ -f /etc/debian_version ]; then
                DISTRO="debian"
            else
                DISTRO="unknown"
            fi
            ;;
        darwin)
            DISTRO="macos"
            ;;
        *)
            echo -e "${RED}Unsupported OS: $OS${NC}"
            exit 1
            ;;
    esac
    
    echo -e "${GREEN}Detected: $OS ($DISTRO) $ARCH${NC}"
}

# Get latest release version
get_latest_version() {
    VERSION=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
    if [ -z "$VERSION" ]; then
        echo -e "${RED}Failed to get latest version${NC}"
        exit 1
    fi
    echo -e "${GREEN}Latest version: $VERSION${NC}"
}

# Check for required commands
check_requirements() {
    for cmd in curl tar; do
        if ! command -v $cmd &> /dev/null; then
            echo -e "${RED}Required command not found: $cmd${NC}"
            exit 1
        fi
    done
}

# Install via Homebrew (macOS)
install_homebrew() {
    echo -e "${YELLOW}Installing via Homebrew...${NC}"
    if ! command -v brew &> /dev/null; then
        echo -e "${RED}Homebrew not found. Please install it first: https://brew.sh${NC}"
        exit 1
    fi
    brew tap BlackOrder/vcdeploy || true
    brew install vcdeploy
    echo -e "${GREEN}Installation complete!${NC}"
}

# Install via DEB package (Debian/Ubuntu)
install_deb() {
    echo -e "${YELLOW}Installing via DEB package...${NC}"
    PACKAGE_URL="https://github.com/$REPO/releases/download/$VERSION/vcdeploy_${VERSION#v}_${ARCH}.deb"
    TMP_DEB=$(mktemp)
    
    curl -fsSL "$PACKAGE_URL" -o "$TMP_DEB"
    
    if command -v sudo &> /dev/null; then
        sudo dpkg -i "$TMP_DEB" || sudo apt-get install -f -y
    else
        dpkg -i "$TMP_DEB" || apt-get install -f -y
    fi
    
    rm -f "$TMP_DEB"
    echo -e "${GREEN}Installation complete!${NC}"
}

# Install via RPM package (RHEL/Fedora/CentOS)
install_rpm() {
    echo -e "${YELLOW}Installing via RPM package...${NC}"
    PACKAGE_URL="https://github.com/$REPO/releases/download/$VERSION/vcdeploy-${VERSION#v}-1.${ARCH}.rpm"
    
    if command -v dnf &> /dev/null; then
        sudo dnf install -y "$PACKAGE_URL"
    elif command -v yum &> /dev/null; then
        sudo yum install -y "$PACKAGE_URL"
    else
        echo -e "${RED}Neither dnf nor yum found${NC}"
        exit 1
    fi
    
    echo -e "${GREEN}Installation complete!${NC}"
}

# Install via tarball (generic)
install_tarball() {
    echo -e "${YELLOW}Installing from tarball...${NC}"
    
    TARBALL_URL="https://github.com/$REPO/releases/download/$VERSION/vcdeploy_${OS}_${ARCH}.tar.gz"
    TMP_DIR=$(mktemp -d)
    
    echo "Downloading from $TARBALL_URL..."
    curl -fsSL "$TARBALL_URL" -o "$TMP_DIR/vcdeploy.tar.gz"
    
    echo "Extracting..."
    tar -xzf "$TMP_DIR/vcdeploy.tar.gz" -C "$TMP_DIR"
    
    echo "Installing binaries to $INSTALL_DIR..."
    if command -v sudo &> /dev/null; then
        sudo mkdir -p "$INSTALL_DIR"
        sudo cp "$TMP_DIR/vcdeploy" "$INSTALL_DIR/"
        sudo cp "$TMP_DIR/vcdeploy-agent" "$INSTALL_DIR/" 2>/dev/null || true
        sudo chmod +x "$INSTALL_DIR/vcdeploy"*
    else
        mkdir -p "$INSTALL_DIR"
        cp "$TMP_DIR/vcdeploy" "$INSTALL_DIR/"
        cp "$TMP_DIR/vcdeploy-agent" "$INSTALL_DIR/" 2>/dev/null || true
        chmod +x "$INSTALL_DIR/vcdeploy"*
    fi
    
    rm -rf "$TMP_DIR"
    echo -e "${GREEN}Installation complete!${NC}"
}

# Create directories and set permissions
setup_directories() {
    echo -e "${YELLOW}Setting up directories...${NC}"
    
    if command -v sudo &> /dev/null; then
        sudo mkdir -p "$CONFIG_DIR" "$DATA_DIR" "$LOG_DIR"
        sudo chmod 755 "$CONFIG_DIR" "$DATA_DIR"
        sudo chmod 750 "$LOG_DIR"
    else
        mkdir -p "$CONFIG_DIR" "$DATA_DIR" "$LOG_DIR"
        chmod 755 "$CONFIG_DIR" "$DATA_DIR"
        chmod 750 "$LOG_DIR"
    fi
}

# Install systemd service (Linux)
install_systemd_service() {
    if [ "$OS" != "linux" ] || ! command -v systemctl &> /dev/null; then
        return
    fi
    
    echo -e "${YELLOW}Installing systemd service...${NC}"
    
    cat << 'EOF' | sudo tee /etc/systemd/system/vcdeploy-master.service > /dev/null
[Unit]
Description=VCDeploy Master Server
After=network.target

[Service]
Type=simple
User=vcdeploy
Group=vcdeploy
ExecStart=/usr/local/bin/vcdeploy master start
ExecReload=/bin/kill -HUP $MAINPID
Restart=always
RestartSec=5
LimitNOFILE=65535
Environment=VCDEPLOY_CONFIG=/etc/vcdeploy/master.yaml

[Install]
WantedBy=multi-user.target
EOF

    cat << 'EOF' | sudo tee /etc/systemd/system/vcdeploy-agent.service > /dev/null
[Unit]
Description=VCDeploy Agent
After=network.target

[Service]
Type=simple
User=vcdeploy
Group=vcdeploy
ExecStart=/usr/local/bin/vcdeploy-agent start
ExecReload=/bin/kill -HUP $MAINPID
Restart=always
RestartSec=5
LimitNOFILE=65535
Environment=VCDEPLOY_AGENT_CONFIG=/etc/vcdeploy/agent.yaml

[Install]
WantedBy=multi-user.target
EOF

    # Create vcdeploy user if it doesn't exist
    if ! id vcdeploy &> /dev/null; then
        sudo useradd -r -s /bin/false -d /var/lib/vcdeploy vcdeploy
    fi
    
    # Set ownership
    sudo chown -R vcdeploy:vcdeploy "$DATA_DIR" "$LOG_DIR"
    
    sudo systemctl daemon-reload
    echo -e "${GREEN}Systemd services installed. Enable with: sudo systemctl enable vcdeploy-master${NC}"
}

# Print post-installation instructions
print_instructions() {
    echo ""
    echo -e "${GREEN}============================================${NC}"
    echo -e "${GREEN}VCDeploy $VERSION installed successfully!${NC}"
    echo -e "${GREEN}============================================${NC}"
    echo ""
    echo "Quick start:"
    echo "  1. Initialize configuration:"
    echo "     vcdeploy init"
    echo ""
    echo "  2. Start the master server:"
    echo "     vcdeploy master start"
    echo ""
    echo "  3. Or use systemd (Linux):"
    echo "     sudo systemctl start vcdeploy-master"
    echo ""
    echo "Documentation: https://github.com/$REPO#readme"
    echo ""
}

# Main installation logic
main() {
    echo -e "${GREEN}VCDeploy Installer${NC}"
    echo ""
    
    check_requirements
    detect_platform
    get_latest_version
    
    # Allow overriding version
    if [ -n "$VCDEPLOY_VERSION" ]; then
        VERSION="$VCDEPLOY_VERSION"
        echo -e "${YELLOW}Using specified version: $VERSION${NC}"
    fi
    
    # Choose installation method
    case "$DISTRO" in
        macos)
            if [ "${USE_TARBALL:-}" = "1" ]; then
                install_tarball
            else
                install_homebrew
            fi
            ;;
        ubuntu|debian|linuxmint|pop)
            if [ "${USE_TARBALL:-}" = "1" ]; then
                install_tarball
            else
                install_deb
            fi
            ;;
        fedora|rhel|centos|rocky|almalinux|amazon)
            if [ "${USE_TARBALL:-}" = "1" ]; then
                install_tarball
            else
                install_rpm
            fi
            ;;
        *)
            echo -e "${YELLOW}Unknown distribution, installing from tarball...${NC}"
            install_tarball
            ;;
    esac
    
    setup_directories
    install_systemd_service
    print_instructions
}

main "$@"
