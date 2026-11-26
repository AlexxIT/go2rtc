#!/bin/sh
# Downloads CDN dependencies from jsdelivr for offline web UI.
# Automatically parses CDN URLs from HTML files, so updating
# a library version in HTML is all that's needed.
#
# Usage: download_cdn.sh <web_dir>
set -e

WEB_DIR="${1:?Usage: download_cdn.sh <web_dir>}"
CDN_DIR="$WEB_DIR/cdn"
mkdir -p "$CDN_DIR"

# Step 1: Extract all jsdelivr CDN URLs from HTML files
URLS=$(grep -roh 'https://cdn\.jsdelivr\.net/npm/[^"'"'"' )`]*' "$WEB_DIR"/*.html | sort -u)

echo "=== Found CDN URLs ==="
echo "$URLS"
echo ""

# Step 2: Process each URL
MONACO_VER=""
for url in $URLS; do
    # Remove CDN prefix to get npm path
    npm_path="${url#https://cdn.jsdelivr.net/npm/}"

    # Extract package@version and file path
    pkg_ver=$(echo "$npm_path" | cut -d/ -f1)
    remaining=$(echo "$npm_path" | cut -d/ -f2-)
    if [ "$remaining" = "$npm_path" ]; then
        file_path=""
    else
        file_path="$remaining"
    fi

    pkg_name=$(echo "$pkg_ver" | sed 's/@[^@]*$//')

    # Monaco editor: remember version, download as tarball later
    case "$pkg_name" in
        monaco-editor)
            MONACO_VER=$(echo "$pkg_ver" | sed 's/.*@//')
            echo "Monaco editor v$MONACO_VER (will download tarball)"
            continue
            ;;
    esac

    # Determine local file path
    if [ -n "$file_path" ]; then
        local_file="$CDN_DIR/$pkg_name/$file_path"
    else
        local_file="$CDN_DIR/$pkg_name/index.js"
    fi

    mkdir -p "$(dirname "$local_file")"
    echo "Downloading $pkg_ver -> $local_file"
    wget -q -O "$local_file" "$url"
done

# Step 3: Download monaco-editor tarball and extract min/ directory
# The AMD loader dynamically loads modules, so we need the entire min/vs/ tree
if [ -n "$MONACO_VER" ]; then
    echo ""
    echo "=== Downloading monaco-editor@$MONACO_VER tarball ==="

    TARBALL_URL=$(wget -q -O - "https://registry.npmjs.org/monaco-editor/$MONACO_VER" | \
        grep -o '"tarball":"[^"]*"' | head -1 | cut -d'"' -f4)

    mkdir -p /tmp/monaco "$CDN_DIR/monaco-editor"
    wget -q -O /tmp/monaco.tgz "$TARBALL_URL"
    tar xzf /tmp/monaco.tgz -C /tmp/monaco

    cp -r /tmp/monaco/package/min "$CDN_DIR/monaco-editor/"
    rm -rf /tmp/monaco /tmp/monaco.tgz

    echo "  Extracted min/ directory ($(du -sh "$CDN_DIR/monaco-editor/min" | cut -f1))"
fi

# Step 4: Patch HTML files to use local paths instead of CDN URLs
echo ""
echo "=== Patching HTML files ==="
for url in $URLS; do
    npm_path="${url#https://cdn.jsdelivr.net/npm/}"

    pkg_ver=$(echo "$npm_path" | cut -d/ -f1)
    remaining=$(echo "$npm_path" | cut -d/ -f2-)
    if [ "$remaining" = "$npm_path" ]; then
        file_path=""
    else
        file_path="$remaining"
    fi

    pkg_name=$(echo "$pkg_ver" | sed 's/@[^@]*$//')

    if [ -n "$file_path" ]; then
        local_url="cdn/$pkg_name/$file_path"
    else
        local_url="cdn/$pkg_name/index.js"
    fi

    echo "  $url -> $local_url"
    sed -i "s|$url|$local_url|g" "$WEB_DIR"/*.html
done

echo ""
echo "=== Done ==="
du -sh "$CDN_DIR"
