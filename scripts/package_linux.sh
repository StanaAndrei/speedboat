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
# 4. Build & Package Master CLI (Linux x86_64)
# -------------------------------------------------------------
echo "📦 Assembling Master Orchestrator CLI executable & bundle..."
if [ ! -f "${BUILD_DIR}/speedboat-master" ]; then
  echo "🔨 Binary build/speedboat-master not found. Building via 'make build-master-cli'..."
  make -C "${REPO_ROOT}" build-master-cli
fi
cp "${BUILD_DIR}/speedboat-master" "${BUILD_DIR}/speedboat-master-linux-amd64"

MASTER_TAR_DIR="${DIST_DIR}/tar-master"
mkdir -p "${MASTER_TAR_DIR}"
cp "${BUILD_DIR}/speedboat-master" "${MASTER_TAR_DIR}/speedboat-master"
chmod 755 "${MASTER_TAR_DIR}/speedboat-master"
cp "${REPO_ROOT}/examples/docker-compose.yaml" "${MASTER_TAR_DIR}/docker-compose.yaml"
if [ -f "${REPO_ROOT}/examples/nats.conf" ]; then
  cp "${REPO_ROOT}/examples/nats.conf" "${MASTER_TAR_DIR}/nats.conf"
fi

tar -czf "${BUILD_DIR}/speedboat-master-linux-amd64.tar.gz" -C "${MASTER_TAR_DIR}" speedboat-master docker-compose.yaml nats.conf
echo "✅ Built: ${BUILD_DIR}/speedboat-master"
echo "✅ Built: ${BUILD_DIR}/speedboat-master-linux-amd64"
echo "✅ Built: ${BUILD_DIR}/speedboat-master-linux-amd64.tar.gz (includes binary, docker-compose.yaml, nats.conf)"

# -------------------------------------------------------------
# 5. Build & Package Agent CLI (Linux x86_64)
# -------------------------------------------------------------
echo "📦 Assembling Headless Agent CLI executable & bundle..."
if [ ! -f "${BUILD_DIR}/speedboat-agent" ]; then
  echo "🔨 Binary build/speedboat-agent not found. Building via 'make build-agent-cli'..."
  make -C "${REPO_ROOT}" build-agent-cli
fi
cp "${BUILD_DIR}/speedboat-agent" "${BUILD_DIR}/speedboat-agent-linux-amd64"

AGENT_TAR_DIR="${DIST_DIR}/tar-agent"
mkdir -p "${AGENT_TAR_DIR}"
cp "${BUILD_DIR}/speedboat-agent" "${AGENT_TAR_DIR}/speedboat-agent"
chmod 755 "${AGENT_TAR_DIR}/speedboat-agent"

cat << EOF > "${AGENT_TAR_DIR}/README.txt"
Speedboat Headless CLI Agent (Linux x86_64) - Version ${VERSION}
==================================================================

Project Speedboat: Universal Volunteer Computing Headless Worker

Running the Agent:
  Execute the binary directly from terminal:
    ./speedboat-agent run --project-id prime-counter --nats-url nats://localhost:4222

More info: https://github.com/StanaAndrei/speedboat
EOF

tar -czf "${BUILD_DIR}/speedboat-agent-linux-amd64.tar.gz" -C "${AGENT_TAR_DIR}" speedboat-agent README.txt
echo "✅ Built: ${BUILD_DIR}/speedboat-agent"
echo "✅ Built: ${BUILD_DIR}/speedboat-agent-linux-amd64"
echo "✅ Built: ${BUILD_DIR}/speedboat-agent-linux-amd64.tar.gz"

# -------------------------------------------------------------
# 6. Generate SHA-256 Checksums
# -------------------------------------------------------------
echo "🔒 Computing SHA-256 checksums..."
cd "${BUILD_DIR}"
CHECKSUM_FILES=("speedboat-linux-amd64.deb" "speedboat-linux-amd64.tar.gz")
[ -f "speedboat-master" ] && CHECKSUM_FILES+=("speedboat-master")
[ -f "speedboat-master-linux-amd64" ] && CHECKSUM_FILES+=("speedboat-master-linux-amd64")
[ -f "speedboat-master-linux-amd64.tar.gz" ] && CHECKSUM_FILES+=("speedboat-master-linux-amd64.tar.gz")
[ -f "speedboat-agent" ] && CHECKSUM_FILES+=("speedboat-agent")
[ -f "speedboat-agent-linux-amd64" ] && CHECKSUM_FILES+=("speedboat-agent-linux-amd64")
[ -f "speedboat-agent-linux-amd64.tar.gz" ] && CHECKSUM_FILES+=("speedboat-agent-linux-amd64.tar.gz")

sha256sum "${CHECKSUM_FILES[@]}" > "${BUILD_DIR}/SHA256SUMS.txt"
cat "${BUILD_DIR}/SHA256SUMS.txt"

# Clean temporary assemble tree
rm -rf "${DIST_DIR}"

echo "============================================================"
echo "🎉 Linux release packaging complete!"
echo "   - ${DEB_OUTPUT}"
echo "   - ${TAR_OUTPUT}"
echo "   - ${BUILD_DIR}/speedboat-master"
echo "   - ${BUILD_DIR}/speedboat-master-linux-amd64"
echo "   - ${BUILD_DIR}/speedboat-master-linux-amd64.tar.gz"
echo "   - ${BUILD_DIR}/speedboat-agent"
echo "   - ${BUILD_DIR}/speedboat-agent-linux-amd64"
echo "   - ${BUILD_DIR}/speedboat-agent-linux-amd64.tar.gz"
echo "   - ${BUILD_DIR}/SHA256SUMS.txt"
echo "============================================================"
