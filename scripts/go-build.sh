#!/bin/bash

set -e

# Version of your application
VERSION=${VERSION:-$(git describe --tags --always --dirty)}
BUILD_DATE=$(date -u +'%Y-%m-%dT%H:%M:%SZ')
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null)
GIT_TREE_STATE=${GIT_TREE_STATE:-dirty}

# Platforms to build for
PLATFORMS=("linux" "darwin")
ARCHITECTURES=("amd64" "arm64")

# Output directory
OUTPUT_DIR="./build"

# Clean and create output directory
rm -rf "${OUTPUT_DIR}"
mkdir -p "${OUTPUT_DIR}"

# Build function
build() {
    local os=$1
    local arch=$2
    local output_name="${OUTPUT_DIR}/server_${os}_${arch}"
    
    if [ "${os}" = "windows" ]; then
        output_name+=".exe"
    fi

    echo "Building for ${os}/${arch}..."
    
    CGO_ENABLED=0 GOOS=${os} GOARCH=${arch} go build \
        -ldflags "-X main.version=${VERSION} \
                 -X main.buildDate=${BUILD_DATE} \
                 -X main.gitCommit=${GIT_COMMIT} \
                 -X main.gitTreeState=${GIT_TREE_STATE}" \
        -o "${output_name}" \
        ./cmd/server

    # Create checksum with just the filename (no path)
    local filename=$(basename "${output_name}")
    (cd "${OUTPUT_DIR}" && shasum -a 256 "${filename}" > "${filename}.sha256")
}

# Build for all platforms and architectures
for os in "${PLATFORMS[@]}"; do
    for arch in "${ARCHITECTURES[@]}"; do
        build "${os}" "${arch}" &
    done
done

# Wait for all builds to complete
wait

echo "Build completed! Output files are in ${OUTPUT_DIR}/"

# Create archives for distribution
for os in "${PLATFORMS[@]}"; do
    for arch in "${ARCHITECTURES[@]}"; do
        bin_name="server_${os}_${arch}"
        if [ "${os}" = "windows" ]; then
            bin_name+=".exe"
        fi
        
        echo "Creating archive for ${os}/${arch}..."
        
        pushd "${OUTPUT_DIR}" > /dev/null
        if [ "${os}" = "windows" ]; then
            zip -r "${bin_name%.exe}_${VERSION}_${os}_${arch}.zip" "${bin_name}" "${bin_name}.sha256"
        else
            tar -czf "${bin_name}_${VERSION}_${os}_${arch}.tar.gz" "${bin_name}" "${bin_name}.sha256"
        fi
        popd > /dev/null
    done
done

echo "Archives created in ${OUTPUT_DIR}/"
ls -lh "${OUTPUT_DIR}/"