#!/bin/bash
# Mac build script
# Usage: chmod +x build_mac.sh && ./build_mac.sh

set -e

echo "📦 Building RollCall for macOS..."

# Check if pyinstaller is installed
if ! command -v pyinstaller &> /dev/null; then
    echo "Installing PyInstaller..."
    pip install pyinstaller
fi

# Install dependencies
echo "Installing dependencies..."
pip install -r requirements.txt

# Create data directory
mkdir -p data

# Build
echo "Building..."
pyinstaller rollcall.spec --clean

# Create dmg folder structure
mkdir -p dist/rollcall-mac
cp -r dist/rollcall/data dist/rollcall-mac/
cp -r dist/rollcall/templates dist/rollcall-mac/
mv dist/rollcall/dist/rollcall.dist/rollcall.app dist/rollcall-mac/

echo ""
echo "✅ Build complete!"
echo "📁 Output: dist/rollcall-mac/"
echo ""
echo "To run:"
echo "  open dist/rollcall-mac/rollcall.app"
echo ""
echo "Or start from terminal:"
echo "  dist/rollcall-mac/rollcall.app/Contents/MacOS/rollcall"
