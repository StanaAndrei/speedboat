#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

RAW_VERSION="${1:-1.0.0}"
# Strip leading 'v' if present (e.g. v1.0.0 -> 1.0.0 for Debian standards)
VERSION="${RAW_VERSION#v}"

echo "============================================================"
echo "📦 Packaging Speedboat Desktop GUI for Linux (x86_64)"
echo "   Version: ${VERSION}"
echo "============================================================"

BUILD_DIR="${REPO_ROOT}/build"
mkdir -p "${BUILD_DIR}"

# 1. Build Linux binary if not already built or needs refresh
if [ ! -f "${BUILD_DIR}/speedboat-gui" ]; then
  echo "🔨 Binary build/speedboat-gui not found. Building via 'make build-linux'..."
  make -C "${REPO_ROOT}" build-linux
fi

DIST_DIR="${BUILD_DIR}/dist"
rm -rf "${DIST_DIR}"
mkdir -p "${DIST_DIR}"

# -------------------------------------------------------------
# 2. Build .deb package
# -------------------------------------------------------------
echo "📦 Assembling Debian (.deb) package..."
DEB_ROOT="${DIST_DIR}/deb-root"
mkdir -p "${DEB_ROOT}/DEBIAN"
mkdir -p "${DEB_ROOT}/usr/bin"
mkdir -p "${DEB_ROOT}/usr/share/applications"
mkdir -p "${DEB_ROOT}/usr/share/icons/hicolor/512x512/apps"

chmod 755 "${DEB_ROOT}"
chmod 755 "${DEB_ROOT}/DEBIAN"
chmod 755 "${DEB_ROOT}/usr" "${DEB_ROOT}/usr/bin" "${DEB_ROOT}/usr/share" "${DEB_ROOT}/usr/share/applications" "${DEB_ROOT}/usr/share/icons" "${DEB_ROOT}/usr/share/icons/hicolor" "${DEB_ROOT}/usr/share/icons/hicolor/512x512" "${DEB_ROOT}/usr/share/icons/hicolor/512x512/apps"

# Install binary
cp "${BUILD_DIR}/speedboat-gui" "${DEB_ROOT}/usr/bin/speedboat-gui"
chmod 755 "${DEB_ROOT}/usr/bin/speedboat-gui"

# Install desktop entry
cat << 'EOF' > "${DEB_ROOT}/usr/share/applications/speedboat-gui.desktop"
[Desktop Entry]
Name=Speedboat
Comment=Speedboat Volunteer Computing Desktop Agent
Exec=/usr/bin/speedboat-gui
Icon=speedboat-gui
Terminal=false
Type=Application
Categories=Utility;Science;DistributedComputing;
Keywords=volunteer;computing;grid;wazero;science;
EOF
chmod 644 "${DEB_ROOT}/usr/share/applications/speedboat-gui.desktop"

# Install icon
if [ -f "${REPO_ROOT}/gui/build/appicon.png" ]; then
  cp "${REPO_ROOT}/gui/build/appicon.png" "${DEB_ROOT}/usr/share/icons/hicolor/512x512/apps/speedboat-gui.png"
  chmod 644 "${DEB_ROOT}/usr/share/icons/hicolor/512x512/apps/speedboat-gui.png"
fi

# Write DEBIAN/control file
cat << EOF > "${DEB_ROOT}/DEBIAN/control"
Package: speedboat-gui
Version: ${VERSION}
Section: utils
Priority: optional
Architecture: amd64
Maintainer: Speedboat Contributors <https://github.com/StanaAndrei/speedboat>
Depends: libgtk-3-0, libwebkit2gtk-4.1-0 | libwebkit2gtk-4.0-37
Description: Speedboat Volunteer Computing Desktop Agent
 Project Speedboat is a modern volunteer computing client that lets users
 contribute idle computer power to scientific simulations with zero friction.
 Powered by Go, Wails, and wazero WebAssembly runtime.
EOF
chmod 644 "${DEB_ROOT}/DEBIAN/control"

DEB_OUTPUT="${BUILD_DIR}/speedboat-linux-amd64.deb"
dpkg-deb --build --root-owner-group "${DEB_ROOT}" "${DEB_OUTPUT}"
echo "✅ Built: ${DEB_OUTPUT}"

# -------------------------------------------------------------
# 3. Build .tar.gz portable archive
# -------------------------------------------------------------
echo "📦 Assembling Universal Linux (.tar.gz) archive..."
TAR_ROOT="${DIST_DIR}/tar-root/speedboat"
mkdir -p "${TAR_ROOT}"

cp "${BUILD_DIR}/speedboat-gui" "${TAR_ROOT}/speedboat-gui"
chmod 755 "${TAR_ROOT}/speedboat-gui"

if [ -f "${REPO_ROOT}/gui/build/appicon.png" ]; then
  cp "${REPO_ROOT}/gui/build/appicon.png" "${TAR_ROOT}/icon.png"
fi

cat << 'EOF' > "${TAR_ROOT}/speedboat.desktop"
[Desktop Entry]
Name=Speedboat
Comment=Speedboat Volunteer Computing Desktop Agent
Exec=./speedboat-gui
Icon=./icon.png
Terminal=false
Type=Application
Categories=Utility;Science;
EOF

cat << EOF > "${TAR_ROOT}/README.txt"
Speedboat Desktop Agent (Linux x86_64) - Version ${VERSION}
=============================================================

Project Speedboat: Universal Volunteer Computing

Requirements:
  - Linux x86_64
  - GTK 3 (libgtk-3)
  - WebKit2GTK 4.1 or 4.0 (libwebkit2gtk-4.1-0 or libwebkit2gtk-4.0-37)

Running the App:
  Directly run the executable from terminal or desktop file manager:
    ./speedboat-gui

More info: https://github.com/StanaAndrei/speedboat
EOF

TAR_OUTPUT="${BUILD_DIR}/speedboat-linux-amd64.tar.gz"
tar -czf "${TAR_OUTPUT}" -C "${DIST_DIR}/tar-root" speedboat
echo "✅ Built: ${TAR_OUTPUT}"

# -------------------------------------------------------------
# 4. Generate SHA-256 Checksums
# -------------------------------------------------------------
echo "🔒 Computing SHA-256 checksums..."
cd "${BUILD_DIR}"
sha256sum "speedboat-linux-amd64.deb" "speedboat-linux-amd64.tar.gz" > "${BUILD_DIR}/SHA256SUMS.txt"
cat "${BUILD_DIR}/SHA256SUMS.txt"

# Clean temporary assemble tree
rm -rf "${DIST_DIR}"

echo "============================================================"
echo "🎉 Linux release packaging complete!"
echo "   - ${DEB_OUTPUT}"
echo "   - ${TAR_OUTPUT}"
echo "   - ${BUILD_DIR}/SHA256SUMS.txt"
echo "============================================================"
