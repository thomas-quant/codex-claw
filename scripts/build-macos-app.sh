#!/bin/bash
# Build macOS .app bundle for codex-claw

set -e

EXECUTABLE=$1

if [ -z "$EXECUTABLE" ]; then
    echo "Usage: $0 <executable>"
    exit 1
fi

EXECUTABLE="codex-claw-${EXECUTABLE}"
echo "executable: $EXECUTABLE"

APP_NAME="Codex Claw"
APP_PATH="./build/${APP_NAME}.app"
APP_CONTENTS="${APP_PATH}/Contents"
APP_MACOS="${APP_CONTENTS}/MacOS"
APP_RESOURCES="${APP_CONTENTS}/Resources"
APP_EXECUTABLE="codex-claw"
ICON_SOURCE="./scripts/icon.icns"

# Clean up existing .app
if [ -d "$APP_PATH" ]; then
    echo "Removing existing ${APP_PATH}"
    rm -rf "$APP_PATH"
fi

# Create directory structure
echo "Creating .app bundle structure..."
mkdir -p "$APP_MACOS"
mkdir -p "$APP_RESOURCES"

# Copy executable
echo "Copying executable..."
if [ -f "./build/${EXECUTABLE}" ]; then
    cp "./build/${EXECUTABLE}" "${APP_MACOS}/${APP_EXECUTABLE}"
else
    echo "Error: ./build/${EXECUTABLE} not found. Please build the main binary first."
    echo "Run: make build"
    exit 1
fi
chmod +x "${APP_MACOS}/"*

# Create Info.plist
echo "Creating Info.plist..."
cat > "${APP_CONTENTS}/Info.plist" << 'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleExecutable</key>
    <string>codex-claw</string>
    <key>CFBundleIdentifier</key>
    <string>com.thomasquant.codexclaw</string>
    <key>CFBundleName</key>
    <string>Codex Claw</string>
    <key>CFBundleDisplayName</key>
    <string>Codex Claw</string>
    <key>CFBundleIconFile</key>
    <string>icon.icns</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleShortVersionString</key>
    <string>1.0</string>
    <key>CFBundleVersion</key>
    <string>1</string>
    <key>NSHighResolutionCapable</key>
    <true/>
    <key>LSMinimumSystemVersion</key>
    <string>10.11</string>
</dict>
</plist>
EOF

#sips -z 128 128 "$ICON_SOURCE" --out "${ICONSET_PATH}/icon_128x128.png" > /dev/null 2>&1
#
## Create icns file
#iconutil -c icns "$ICONSET_PATH" -o "$ICON_OUTPUT" 2>/dev/null || {
#    echo "Warning: iconutil failed"
#}

cp "$ICON_SOURCE" "${APP_RESOURCES}/icon.icns"

echo ""
echo "=========================================="
echo "Successfully created: ${APP_PATH}"
echo "=========================================="
echo ""
echo "To launch Codex Claw:"
echo "  1. Double-click ${APP_NAME}.app in Finder"
echo "  2. Or use: open ${APP_PATH}"
echo ""
echo "Note: The app launches as a standard macOS application bundle."
echo ""
