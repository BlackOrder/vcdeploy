#!/bin/bash
# VCDeploy Uninstall Script
# This script removes all vcdeploy components from the system.

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
VCDEPLOY_USER="vcdeploy"
VCDEPLOY_GROUP="vcdeploy"
CONFIG_DIR="/etc/vcdeploy"
DATA_DIR="/var/lib/vcdeploy"
LOG_DIR="/var/log/vcdeploy"
RUN_DIR="/var/run/vcdeploy"
BIN_DIR="/usr/local/bin"
CA_DIR="${DATA_DIR}/ca"
SSH_DIR="${DATA_DIR}/ssh"

# Print colored output
print_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if running as root
check_root() {
    if [[ $EUID -ne 0 ]]; then
        print_error "This script must be run as root (use sudo)"
        exit 1
    fi
}

# Confirm uninstallation
confirm_uninstall() {
    echo ""
    print_warn "This will completely remove VCDeploy from your system."
    print_warn "The following will be deleted:"
    echo "  - Systemd services (vcdeploy-master, vcdeploy-agent)"
    echo "  - Configuration files (${CONFIG_DIR})"
    echo "  - Data files (${DATA_DIR})"
    echo "  - Log files (${LOG_DIR})"
    echo "  - CA certificates and keys"
    echo "  - SSH deploy keys"
    echo "  - vcdeploy user and group"
    echo "  - Binary files"
    echo ""
    
    if [[ "${FORCE_UNINSTALL}" != "true" ]]; then
        read -p "Are you sure you want to continue? [y/N] " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            print_info "Uninstallation cancelled."
            exit 0
        fi
    fi
}

# Stop and disable systemd services
remove_services() {
    print_info "Stopping and removing systemd services..."
    
    # Stop services if running
    for service in vcdeploy-master vcdeploy-agent; do
        if systemctl is-active --quiet "$service" 2>/dev/null; then
            print_info "Stopping $service..."
            systemctl stop "$service" || true
        fi
        
        if systemctl is-enabled --quiet "$service" 2>/dev/null; then
            print_info "Disabling $service..."
            systemctl disable "$service" || true
        fi
    done
    
    # Remove service files
    rm -f /etc/systemd/system/vcdeploy-master.service
    rm -f /etc/systemd/system/vcdeploy-agent.service
    rm -f /usr/lib/systemd/system/vcdeploy-master.service
    rm -f /usr/lib/systemd/system/vcdeploy-agent.service
    
    # Reload systemd
    systemctl daemon-reload || true
    
    print_info "Systemd services removed."
}

# Remove CA certificates
remove_ca_certs() {
    print_info "Removing CA certificates..."
    
    # Remove from system trust store
    if command -v update-ca-trust &> /dev/null; then
        # RHEL/CentOS/Fedora
        rm -f /etc/pki/ca-trust/source/anchors/vcdeploy-ca.crt
        update-ca-trust extract || true
    elif command -v update-ca-certificates &> /dev/null; then
        # Debian/Ubuntu
        rm -f /usr/local/share/ca-certificates/vcdeploy-ca.crt
        update-ca-certificates || true
    fi
    
    # Remove CA directory
    if [[ -d "${CA_DIR}" ]]; then
        rm -rf "${CA_DIR}"
        print_info "CA directory removed: ${CA_DIR}"
    fi
}

# Remove SSH keys
remove_ssh_keys() {
    print_info "Removing SSH deploy keys..."
    
    if [[ -d "${SSH_DIR}" ]]; then
        # Securely delete private keys
        find "${SSH_DIR}" -name "*.key" -type f -exec shred -u {} \; 2>/dev/null || \
            rm -f "${SSH_DIR}"/*.key
        rm -rf "${SSH_DIR}"
        print_info "SSH directory removed: ${SSH_DIR}"
    fi
    
    # Remove from known_hosts
    if [[ -f "/root/.ssh/known_hosts" ]]; then
        print_info "Note: You may want to clean /root/.ssh/known_hosts manually"
    fi
}

# Remove configuration files
remove_config() {
    print_info "Removing configuration files..."
    
    if [[ -d "${CONFIG_DIR}" ]]; then
        rm -rf "${CONFIG_DIR}"
        print_info "Configuration directory removed: ${CONFIG_DIR}"
    fi
    
    # Remove any environment files
    rm -f /etc/default/vcdeploy
    rm -f /etc/sysconfig/vcdeploy
}

# Remove data files
remove_data() {
    print_info "Removing data files..."
    
    if [[ -d "${DATA_DIR}" ]]; then
        rm -rf "${DATA_DIR}"
        print_info "Data directory removed: ${DATA_DIR}"
    fi
    
    if [[ -d "${RUN_DIR}" ]]; then
        rm -rf "${RUN_DIR}"
        print_info "Run directory removed: ${RUN_DIR}"
    fi
}

# Remove log files
remove_logs() {
    print_info "Removing log files..."
    
    if [[ -d "${LOG_DIR}" ]]; then
        rm -rf "${LOG_DIR}"
        print_info "Log directory removed: ${LOG_DIR}"
    fi
    
    # Remove logrotate config if exists
    rm -f /etc/logrotate.d/vcdeploy
}

# Remove binaries
remove_binaries() {
    print_info "Removing binary files..."
    
    rm -f "${BIN_DIR}/vcdeploy"
    rm -f "${BIN_DIR}/vcdeploy-master"
    rm -f "${BIN_DIR}/vcdeploy-agent"
    
    # Remove from PATH if using alternatives
    if command -v update-alternatives &> /dev/null; then
        update-alternatives --remove vcdeploy "${BIN_DIR}/vcdeploy" 2>/dev/null || true
    fi
}

# Remove user and group
remove_user() {
    print_info "Removing vcdeploy user and group..."
    
    # Kill any remaining processes
    pkill -u "${VCDEPLOY_USER}" 2>/dev/null || true
    
    # Remove user
    if id "${VCDEPLOY_USER}" &>/dev/null; then
        userdel "${VCDEPLOY_USER}" 2>/dev/null || \
            userdel -f "${VCDEPLOY_USER}" 2>/dev/null || true
        print_info "User removed: ${VCDEPLOY_USER}"
    fi
    
    # Remove group
    if getent group "${VCDEPLOY_GROUP}" &>/dev/null; then
        groupdel "${VCDEPLOY_GROUP}" 2>/dev/null || true
        print_info "Group removed: ${VCDEPLOY_GROUP}"
    fi
}

# Remove firewall rules
remove_firewall() {
    print_info "Removing firewall rules..."
    
    # firewalld (RHEL/CentOS/Fedora)
    if command -v firewall-cmd &> /dev/null; then
        firewall-cmd --permanent --remove-port=9000/tcp 2>/dev/null || true
        firewall-cmd --permanent --remove-port=9001/tcp 2>/dev/null || true
        firewall-cmd --reload 2>/dev/null || true
    fi
    
    # ufw (Ubuntu)
    if command -v ufw &> /dev/null; then
        ufw delete allow 9000/tcp 2>/dev/null || true
        ufw delete allow 9001/tcp 2>/dev/null || true
    fi
}

# Clean up database (if using SQLite)
cleanup_database() {
    print_info "Cleaning up database..."
    
    # SQLite databases are in DATA_DIR, already removed
    # If using PostgreSQL or MySQL, print instructions
    if [[ -f "${CONFIG_DIR}/master.yaml" ]]; then
        if grep -q "postgres" "${CONFIG_DIR}/master.yaml" 2>/dev/null; then
            print_warn "PostgreSQL database detected. Manual cleanup may be required:"
            echo "  DROP DATABASE vcdeploy;"
            echo "  DROP USER vcdeploy;"
        fi
        if grep -q "mysql" "${CONFIG_DIR}/master.yaml" 2>/dev/null; then
            print_warn "MySQL database detected. Manual cleanup may be required:"
            echo "  DROP DATABASE vcdeploy;"
            echo "  DROP USER 'vcdeploy'@'localhost';"
        fi
    fi
}

# Show completion message
show_complete() {
    echo ""
    print_info "==================================="
    print_info "VCDeploy has been uninstalled."
    print_info "==================================="
    echo ""
    print_warn "Note: If you had external database connections, please clean them up manually."
    print_warn "Note: Deployed application files were NOT removed from target servers."
    echo ""
}

# Main function
main() {
    echo "========================================"
    echo "   VCDeploy Uninstallation Script"
    echo "========================================"
    echo ""
    
    check_root
    confirm_uninstall
    
    # Perform uninstallation in order
    remove_services
    remove_firewall
    remove_ca_certs
    remove_ssh_keys
    cleanup_database
    remove_config
    remove_data
    remove_logs
    remove_binaries
    remove_user
    
    show_complete
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -f|--force)
            FORCE_UNINSTALL="true"
            shift
            ;;
        -h|--help)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  -f, --force    Skip confirmation prompt"
            echo "  -h, --help     Show this help message"
            exit 0
            ;;
        *)
            print_error "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Run main
main
