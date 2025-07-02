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
    (cd "${OUTPUT_DIR}" && shasum -a 256 "${filename}" >> SHA256SUMS.tmp)
}

# Build for all platforms and architectures
for os in "${PLATFORMS[@]}"; do
    for arch in "${ARCHITECTURES[@]}"; do
        build "${os}" "${arch}" &
    done
done

# Wait for all builds to complete
wait

# Sort and deduplicate the checksums file
if [ -f "${OUTPUT_DIR}/SHA256SUMS.tmp" ]; then
    sort -u "${OUTPUT_DIR}/SHA256SUMS.tmp" > "${OUTPUT_DIR}/SHA256SUMS"
    rm -f "${OUTPUT_DIR}/SHA256SUMS.tmp"
    echo "Created ${OUTPUT_DIR}/SHA256SUMS"
fi

echo "Build completed! Output files are in ${OUTPUT_DIR}/"

# List the built files
echo -e "\nBuild output in ${OUTPUT_DIR}/:"
ls -lh "${OUTPUT_DIR}/"
echo -e "\nChecksums in ${OUTPUT_DIR}/SHA256SUMS:"
cat "${OUTPUT_DIR}/SHA256SUMS"