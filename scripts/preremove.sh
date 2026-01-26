#!/bin/bash
# Pre-removal script for VCDeploy packages

set -e

# Stop services before removal
if command -v systemctl &> /dev/null; then
    systemctl stop vcdeploy-master 2>/dev/null || true
    systemctl stop vcdeploy-agent 2>/dev/null || true
    systemctl disable vcdeploy-master 2>/dev/null || true
    systemctl disable vcdeploy-agent 2>/dev/null || true
fi

# macOS launchd
if command -v launchctl &> /dev/null; then
    launchctl unload /Library/LaunchDaemons/com.blackorder.vcdeploy.master.plist 2>/dev/null || true
    launchctl unload /Library/LaunchDaemons/com.blackorder.vcdeploy.agent.plist 2>/dev/null || true
    launchctl unload ~/Library/LaunchAgents/com.blackorder.vcdeploy.*.plist 2>/dev/null || true
fi

# OpenRC
if command -v rc-service &> /dev/null; then
    rc-service vcdeploy-master stop 2>/dev/null || true
    rc-service vcdeploy-agent stop 2>/dev/null || true
    rc-update del vcdeploy-master 2>/dev/null || true
    rc-update del vcdeploy-agent 2>/dev/null || true
fi

echo "VCDeploy services stopped."

exit 0
