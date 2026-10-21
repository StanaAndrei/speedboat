#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

RAW_VERSION="${1:-1.0.0}"
VERSION="${RAW_VERSION#v}"

echo "============================================================"
echo "📦 Packaging Speedboat Desktop GUI for Windows (x86_64)"
echo "   Version: ${VERSION}"
echo "============================================================"

BUILD_DIR="${REPO_ROOT}/build"
mkdir -p "${BUILD_DIR}"

EXE_OUTPUT="${BUILD_DIR}/speedboat-windows-amd64.exe"

# 1. Build Windows binary if not already present
if [ ! -f "${EXE_OUTPUT}" ]; then
  if ! command -v x86_64-w64-mingw32-gcc >/dev/null 2>&1; then
    echo "❌ Error: x86_64-w64-mingw32-gcc is required to cross-compile for Windows."
    echo "   Install it via: sudo apt-get install -y gcc-mingw-w64-x86-64"
    exit 1
  fi

  echo "🔨 Cross-compiling Windows binary via Wails + MinGW-w64..."
  cd "${REPO_ROOT}/gui"
  CC=x86_64-w64-mingw32-gcc CGO_ENABLED=1 wails build -platform windows/amd64 -o speedboat-windows-amd64.exe
  cp "${REPO_ROOT}/gui/build/bin/speedboat-windows-amd64.exe" "${EXE_OUTPUT}"
fi

echo "✅ Windows binary ready: ${EXE_OUTPUT}"

# -------------------------------------------------------------
# 2. Build .zip portable archive
# -------------------------------------------------------------
echo "📦 Assembling Universal Windows (.zip) archive..."
DIST_DIR="${BUILD_DIR}/dist-win"
rm -rf "${DIST_DIR}"
mkdir -p "${DIST_DIR}/speedboat"

cp "${EXE_OUTPUT}" "${DIST_DIR}/speedboat/speedboat.exe"

cat << EOF > "${DIST_DIR}/speedboat/README.txt"
Speedboat Desktop Agent (Windows 10 / 11 64-bit) - Version ${VERSION}
=====================================================================

Project Speedboat: Universal Volunteer Computing

Requirements:
  - Windows 10 (version 1809+) or Windows 11 64-bit
  - Microsoft Edge WebView2 Runtime (Pre-installed on Windows 10/11)

Running the App:
  Double-click "speedboat.exe" to start computing.
  No administrative rights, drivers, or Docker installation required.

More info: https://github.com/StanaAndrei/speedboat
EOF

ZIP_OUTPUT="${BUILD_DIR}/speedboat-windows-amd64.zip"
rm -f "${ZIP_OUTPUT}"

if command -v zip >/dev/null 2>&1; then
  (cd "${DIST_DIR}" && zip -r "${ZIP_OUTPUT}" speedboat)
else
  # Fallback to python or tar if zip command is absent
  python3 -c "import shutil; shutil.make_archive('${BUILD_DIR}/speedboat-windows-amd64', 'zip', '${DIST_DIR}', 'speedboat')"
fi

echo "✅ Built: ${ZIP_OUTPUT}"

# -------------------------------------------------------------
# 3. Compute / Append SHA-256 Checksums
# -------------------------------------------------------------
echo "🔒 Computing SHA-256 checksums..."
cd "${BUILD_DIR}"

# Remove existing entries for Windows assets in SHA256SUMS.txt if present
if [ -f "SHA256SUMS.txt" ]; then
  grep -v "speedboat-windows-amd64" "SHA256SUMS.txt" > "SHA256SUMS.tmp" || true
  mv "SHA256SUMS.tmp" "SHA256SUMS.txt"
fi

sha256sum "speedboat-windows-amd64.exe" "speedboat-windows-amd64.zip" >> "SHA256SUMS.txt"
cat "SHA256SUMS.txt"

rm -rf "${DIST_DIR}"

echo "============================================================"
echo "🎉 Windows release packaging complete!"
echo "   - ${EXE_OUTPUT}"
echo "   - ${ZIP_OUTPUT}"
echo "============================================================"
