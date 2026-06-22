#!/bin/sh
# c3x installer. Detects your OS/arch, downloads the matching release
# archive from GitHub, verifies its checksum, and installs the binary.
#
#   curl -fsSL https://c3x.dev/install.sh | sh
#
# Env overrides:
#   C3X_VERSION   pin a version (e.g. v0.1.0); default: latest release
#   C3X_INSTALL_DIR  install location; default: /usr/local/bin (falls
#                    back to ~/.local/bin when not writable)
set -eu

REPO="c3xdev/c3x"
BIN="c3x"

err() { printf 'c3x install: %s\n' "$1" >&2; exit 1; }
have() { command -v "$1" >/dev/null 2>&1; }

# --- detect platform ---------------------------------------------------
os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  linux) os=linux ;;
  darwin) os=darwin ;;
  *) err "unsupported OS '$os' (Linux and macOS only; on Windows use the release zip)" ;;
esac

arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) err "unsupported architecture '$arch'" ;;
esac

# --- pick a downloader -------------------------------------------------
if have curl; then dl() { curl -fsSL "$1"; }; dlo() { curl -fsSL -o "$2" "$1"; }
elif have wget; then dl() { wget -qO- "$1"; }; dlo() { wget -qO "$2" "$1"; }
else err "need curl or wget"; fi

# --- resolve version ---------------------------------------------------
version="${C3X_VERSION:-}"
if [ -z "$version" ]; then
  version=$(dl "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' | head -1 | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')
  [ -n "$version" ] || err "could not resolve the latest release; set C3X_VERSION"
fi
ver_no_v=$(printf '%s' "$version" | sed 's/^v//')

# goreleaser archive name: c3x_<version>_<os>_<arch>.tar.gz
archive="${BIN}_${ver_no_v}_${os}_${arch}.tar.gz"
base="https://github.com/${REPO}/releases/download/${version}"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

printf 'Downloading %s %s (%s/%s)...\n' "$BIN" "$version" "$os" "$arch"
dlo "${base}/${archive}" "${tmp}/${archive}" || err "download failed: ${base}/${archive}"

# --- verify checksum (best effort) ------------------------------------
if dlo "${base}/checksums.txt" "${tmp}/checksums.txt" 2>/dev/null; then
  if have sha256sum; then sum=$(sha256sum "${tmp}/${archive}" | awk '{print $1}')
  elif have shasum; then sum=$(shasum -a 256 "${tmp}/${archive}" | awk '{print $1}')
  else sum=""; fi
  if [ -n "$sum" ]; then
    grep -q "$sum" "${tmp}/checksums.txt" || err "checksum mismatch for ${archive}"
    printf 'Checksum verified.\n'
  fi
fi

# --- extract + install -------------------------------------------------
tar -xzf "${tmp}/${archive}" -C "$tmp" || err "extract failed"
[ -f "${tmp}/${BIN}" ] || err "binary not found in archive"
chmod +x "${tmp}/${BIN}"

dir="${C3X_INSTALL_DIR:-/usr/local/bin}"
if [ -w "$dir" ] 2>/dev/null || ( [ -d "$dir" ] && touch "${dir}/.c3x_w" 2>/dev/null && rm -f "${dir}/.c3x_w" ); then
  mv "${tmp}/${BIN}" "${dir}/${BIN}"
elif have sudo; then
  printf 'Installing to %s (needs sudo)...\n' "$dir"
  sudo mv "${tmp}/${BIN}" "${dir}/${BIN}"
else
  dir="${HOME}/.local/bin"
  mkdir -p "$dir"
  mv "${tmp}/${BIN}" "${dir}/${BIN}"
  printf 'Installed to %s — add it to your PATH if it is not already.\n' "$dir"
fi

printf '\nInstalled %s to %s/%s\n' "$BIN" "$dir" "$BIN"
"${dir}/${BIN}" --version 2>/dev/null || true
printf 'Run: c3x estimate --path .\n'
