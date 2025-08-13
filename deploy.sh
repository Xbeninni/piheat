#!/bin/bash

# Pi Temperature Monitor Deployment Script
# This script will deploy the piheat application as a systemd service

set -e  # Exit on any error

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_header() {
    echo -e "${BLUE}[DEPLOY]${NC} $1"
}

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

print_header "Pi Temperature Monitor Deployment"
print_status "Starting deployment from: $SCRIPT_DIR"

# Check if running as root for service installation
if [[ $EUID -eq 0 ]]; then
    print_warning "Running as root. Service will be installed system-wide."
    SERVICE_DIR="/etc/systemd/system"
    SERVICE_USER="piheat"
    INSTALL_DIR="/opt/piheat"
else
    print_status "Running as regular user. Service will be installed for current user."
    SERVICE_DIR="$HOME/.config/systemd/user"
    SERVICE_USER="$USER"
    INSTALL_DIR="$HOME/.local/piheat"
    mkdir -p "$SERVICE_DIR"
fi

# Detect architecture
ARCH=$(uname -m)
case $ARCH in
    x86_64)
        BINARY_NAME="piheat"
        GO_ARCH="amd64"
        ;;
    aarch64|arm64)
        BINARY_NAME="piheat-arm64"
        GO_ARCH="arm64"
        ;;
    armv7l|armv6l)
        BINARY_NAME="piheat-arm64"
        GO_ARCH="arm"
        ;;
    *)
        print_warning "Unknown architecture: $ARCH, defaulting to amd64"
        BINARY_NAME="piheat"
        GO_ARCH="amd64"
        ;;
esac

print_status "Detected architecture: $ARCH, using binary: $BINARY_NAME"

# Create installation directory
print_status "Creating installation directory: $INSTALL_DIR"
if [[ $EUID -eq 0 ]]; then
    mkdir -p "$INSTALL_DIR"
else
    mkdir -p "$INSTALL_DIR"
fi

# Check if binary exists
if [[ ! -f "$BINARY_NAME" ]]; then
    print_warning "Binary $BINARY_NAME not found. Building from source..."
    
    # Check if Go is installed
    if ! command -v go &> /dev/null; then
        print_warning "Go is not installed. Installing Go..."
        
        # Detect OS
        OS=$(uname -s | tr '[:upper:]' '[:lower:]')
        
        # Download and install Go
        GO_VERSION="1.21.5"
        GO_TARBALL="go${GO_VERSION}.${OS}-${GO_ARCH}.tar.gz"
        GO_URL="https://go.dev/dl/${GO_TARBALL}"
        
        print_status "Downloading Go ${GO_VERSION} for ${OS}-${GO_ARCH}..."
        
        # Create temporary directory
        TMP_DIR=$(mktemp -d)
        cd "$TMP_DIR"
        
        # Download Go
        curl -fsSL "$GO_URL" -o "$GO_TARBALL"
        
        # Install Go
        if [[ $EUID -eq 0 ]]; then
            # System-wide installation
            rm -rf /usr/local/go
            tar -C /usr/local -xzf "$GO_TARBALL"
            
            # Add to system PATH
            if ! grep -q "/usr/local/go/bin" /etc/profile; then
                echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile
            fi
            
            export PATH=$PATH:/usr/local/go/bin
        else
            # User installation
            rm -rf "$HOME/go-install"
            mkdir -p "$HOME/go-install"
            tar -C "$HOME/go-install" -xzf "$GO_TARBALL"
            
            # Add to user PATH
            if ! grep -q "go-install/go/bin" "$HOME/.bashrc"; then
                echo 'export PATH=$PATH:$HOME/go-install/go/bin' >> "$HOME/.bashrc"
            fi
            
            export PATH=$PATH:$HOME/go-install/go/bin
        fi
        
        # Clean up
        cd "$SCRIPT_DIR"
        rm -rf "$TMP_DIR"
        
        print_status "Go installed successfully!"
    else
        print_status "Go is already installed: $(go version)"
    fi
    
    # Ensure go.mod exists
    if [[ ! -f "go.mod" ]]; then
        print_error "go.mod not found. Please ensure you're in the correct directory."
        exit 1
    fi
    
    # Install dependencies and build
    print_status "Installing dependencies..."
    go mod download
    go mod tidy
    
    print_status "Building binary for $GO_ARCH..."
    CGO_ENABLED=1 GOOS=linux GOARCH=$GO_ARCH go build -o "$BINARY_NAME" main.go
    
    if [[ ! -f "$BINARY_NAME" ]]; then
        print_error "Failed to build binary"
        exit 1
    fi
    
    print_status "Binary built successfully: $BINARY_NAME"
else
    print_status "Binary $BINARY_NAME already exists"
fi

# Make binary executable
chmod +x "$BINARY_NAME"

# Copy files to installation directory
print_status "Installing files to $INSTALL_DIR"
cp "$BINARY_NAME" "$INSTALL_DIR/"
if [[ -f "temperature.db" ]]; then
    cp "temperature.db" "$INSTALL_DIR/"
    print_status "Existing database copied to installation directory"
fi

# Create user if running as root and user doesn't exist
if [[ $EUID -eq 0 ]] && [[ "$SERVICE_USER" != "root" ]]; then
    if ! id "$SERVICE_USER" &>/dev/null; then
        print_status "Creating user: $SERVICE_USER"
        useradd --system --home-dir "$INSTALL_DIR" --shell /bin/false "$SERVICE_USER"
    fi
fi

# Create systemd service file
SERVICE_FILE="$SERVICE_DIR/piheat.service"

print_status "Creating systemd service file: $SERVICE_FILE"

if [[ $EUID -eq 0 ]]; then
    # System service
    cat > "$SERVICE_FILE" << EOF
[Unit]
Description=Pi Temperature Monitor
Documentation=https://github.com/Xbeninni/piheat
After=network.target
Wants=network.target

[Service]
Type=simple
User=$SERVICE_USER
Group=$SERVICE_USER
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/$BINARY_NAME
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=piheat

# Security settings
NoNewPrivileges=yes
PrivateTmp=yes
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths=$INSTALL_DIR
ProtectKernelTunables=yes
ProtectKernelModules=yes
ProtectControlGroups=yes

[Install]
WantedBy=multi-user.target
EOF

    # Set ownership
    chown "$SERVICE_USER:$SERVICE_USER" "$INSTALL_DIR" -R
    
    # Reload systemd and enable service
    systemctl daemon-reload
    systemctl enable piheat.service
    
    # Stop service if running
    if systemctl is-active --quiet piheat.service; then
        print_status "Stopping existing service..."
        systemctl stop piheat.service
    fi
    
    # Start service
    print_status "Starting piheat service..."
    systemctl start piheat.service
    
    # Check status
    sleep 2
    if systemctl is-active --quiet piheat.service; then
        print_status "Service started successfully!"
        systemctl status piheat.service --no-pager -l
    else
        print_error "Service failed to start!"
        systemctl status piheat.service --no-pager -l
        exit 1
    fi
    
else
    # User service
    cat > "$SERVICE_FILE" << EOF
[Unit]
Description=Pi Temperature Monitor (User)
Documentation=https://github.com/Xbeninni/piheat
After=default.target

[Service]
Type=simple
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/$BINARY_NAME
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=default.target
EOF

    # Enable lingering for user to keep service running after logout
    if command -v loginctl &> /dev/null; then
        print_status "Enabling lingering for user $USER to keep service running after logout..."
        sudo loginctl enable-linger "$USER" || print_warning "Could not enable lingering. Service may stop on logout."
    else
        print_warning "loginctl not available. User service may stop on logout."
    fi
    
    # Reload systemd and enable service
    systemctl --user daemon-reload
    systemctl --user enable piheat.service
    
    # Stop service if running
    if systemctl --user is-active --quiet piheat.service; then
        print_status "Stopping existing service..."
        systemctl --user stop piheat.service
    fi
    
    # Start service
    print_status "Starting piheat service..."
    systemctl --user start piheat.service
    
    # Check status
    sleep 2
    if systemctl --user is-active --quiet piheat.service; then
        print_status "Service started successfully!"
        systemctl --user status piheat.service --no-pager -l
    else
        print_error "Service failed to start!"
        systemctl --user status piheat.service --no-pager -l
        exit 1
    fi
fi

# Create management scripts
print_status "Creating management scripts..."

# Start script
cat > start.sh << 'EOF'
#!/bin/bash
if [[ $EUID -eq 0 ]]; then
    systemctl start piheat.service
    systemctl status piheat.service --no-pager
else
    systemctl --user start piheat.service
    systemctl --user status piheat.service --no-pager
fi
EOF

# Stop script
cat > stop.sh << 'EOF'
#!/bin/bash
if [[ $EUID -eq 0 ]]; then
    systemctl stop piheat.service
else
    systemctl --user stop piheat.service
fi
echo "PiHeat service stopped"
EOF

# Status script
cat > status.sh << 'EOF'
#!/bin/bash
if [[ $EUID -eq 0 ]]; then
    systemctl status piheat.service --no-pager -l
else
    systemctl --user status piheat.service --no-pager -l
fi
EOF

# Logs script
cat > logs.sh << 'EOF'
#!/bin/bash
if [[ $EUID -eq 0 ]]; then
    journalctl -u piheat.service -f
else
    journalctl --user -u piheat.service -f
fi
EOF

# Uninstall script
cat > uninstall.sh << 'EOF'
#!/bin/bash
set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

print_status() { echo -e "${GREEN}[INFO]${NC} $1"; }
print_warning() { echo -e "${YELLOW}[WARN]${NC} $1"; }

print_warning "This will completely remove the PiHeat service and all related files."
read -p "Are you sure you want to continue? (y/N): " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Aborted."
    exit 1
fi

if [[ $EUID -eq 0 ]]; then
    print_status "Stopping and disabling system service..."
    systemctl stop piheat.service 2>/dev/null || true
    systemctl disable piheat.service 2>/dev/null || true
    rm -f /etc/systemd/system/piheat.service
    systemctl daemon-reload
else
    print_status "Stopping and disabling user service..."
    systemctl --user stop piheat.service 2>/dev/null || true
    systemctl --user disable piheat.service 2>/dev/null || true
    rm -f "$HOME/.config/systemd/user/piheat.service"
    systemctl --user daemon-reload
fi

print_status "Service uninstalled successfully!"
print_warning "Note: Binary files and data are kept. Remove manually if needed."
EOF

# Make scripts executable
chmod +x start.sh stop.sh status.sh logs.sh uninstall.sh

print_header "Deployment Complete!"
echo ""
print_status "PiHeat has been deployed as a systemd service!"
echo ""
print_status "Service Management:"
if [[ $EUID -eq 0 ]]; then
    echo "  Start:     systemctl start piheat.service    (or ./start.sh)"
    echo "  Stop:      systemctl stop piheat.service     (or ./stop.sh)"
    echo "  Status:    systemctl status piheat.service   (or ./status.sh)"
    echo "  Logs:      journalctl -u piheat.service -f   (or ./logs.sh)"
    echo "  Restart:   systemctl restart piheat.service"
    echo "  Uninstall: ./uninstall.sh"
else
    echo "  Start:     systemctl --user start piheat.service    (or ./start.sh)"
    echo "  Stop:      systemctl --user stop piheat.service     (or ./stop.sh)"
    echo "  Status:    systemctl --user status piheat.service   (or ./status.sh)"
    echo "  Logs:      journalctl --user -u piheat.service -f   (or ./logs.sh)"
    echo "  Restart:   systemctl --user restart piheat.service"
    echo "  Uninstall: ./uninstall.sh"
fi
echo ""
print_status "Web Interface: http://localhost:8082"
echo ""
print_status "Installation Directory: $INSTALL_DIR"
print_status "Database: $INSTALL_DIR/temperature.db"
print_status "Binary: $INSTALL_DIR/$BINARY_NAME"
echo ""