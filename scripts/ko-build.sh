#!/bin/bash

set -e

# Default values
DOCKER_REGISTRY=""
IMAGE_NAME="locnh/cachetf"
TAG=${TAG:-$(git describe --tags --always --dirty)}
PLATFORMS="linux/amd64,linux/arm64"
PUSH=false
LOAD=true

# Help function
show_help() {
    cat << 'EOF'
Usage: ./ko-build.sh [OPTIONS]
Build and optionally push container images using ko.

Options:
  --registry=REGISTRY    Docker registry URL (e.g., ghcr.io/username)
  --image=NAME           Image name (default: locnh/cachetf)
  --tag=TAG              Image tag (default: git describe --tags --always --dirty)
  --platforms=PLATFORMS  Comma-separated list of platforms (default: linux/amd64,linux/arm64)
  --push                 Push the image to the registry after building
  --no-load              Skip loading the image into the local Docker daemon
  -h, --help             Show this help message and exit

Examples:
  # Build for local development (single platform)
  ./ko-build.sh

  # Build and push multi-arch image
  ./ko-build.sh --push

  # Build with custom registry and tag
  ./ko-build.sh --registry=docker.io --tag=v1.0.0 --push

  # Build for specific platforms
  ./ko-build.sh --platforms=linux/arm64

  # Build without loading to local Docker
  ./ko-build.sh --no-load
EOF
    exit 0
}

# Show help if no arguments or --help is provided
if [ $# -eq 0 ] || [[ " $* " == *" --help "* ]] || [[ " $* " == *" -h "* ]]; then
    show_help
fi

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --registry=*)
            DOCKER_REGISTRY="${1#*=}"
            shift
            ;;
        --image=*)
            IMAGE_NAME="${1#*=}"
            shift
            ;;
        --tag=*)
            TAG="${1#*=}"
            shift
            ;;
        --platforms=*)
            PLATFORMS="${1#*=}"
            shift
            ;;
        --push)
            PUSH=true
            LOAD=false
            shift
            ;;
        --no-load)
            LOAD=false
            shift
            ;;
        -h|--help)
            show_help
            ;;
        *)
            echo "Error: Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

# Set the full image name
if [ -n "$DOCKER_REGISTRY" ]; then
    FULL_IMAGE_NAME="${DOCKER_REGISTRY}/${IMAGE_NAME}"
else
    FULL_IMAGE_NAME="${IMAGE_NAME}"
fi

echo "Building container image ${FULL_IMAGE_NAME}:${TAG} for platforms: ${PLATFORMS}"

# Ensure ko is available
if ! command -v ko &> /dev/null; then
    echo "ko is required but not installed. Please install it first."
    echo "See: https://github.com/ko-build/ko#install"
    exit 1
fi

# Prepare ko environment
export KO_DOCKER_REPO="$FULL_IMAGE_NAME"

# Build command arguments
KO_ARGS=(
    "--bare"
    "--platform=$PLATFORMS"
    "--image-label=org.opencontainers.image.version=$TAG"
    "--image-label=org.opencontainers.image.revision=$(git rev-parse HEAD)"
    "--image-label=org.opencontainers.image.created=$(date -u +'%Y-%m-%dT%H:%M:%SZ')"
    "--tags=$TAG"  # Explicitly set the tag
)

# Add push flag if needed
if [ "$PUSH" = true ]; then
    KO_ARGS+=("--push")
else
    KO_ARGS+=("--local")
    
    # For local builds, we can only handle one platform at a time
    if [[ "$PLATFORMS" == *,* ]]; then
        echo "Warning: Cannot load multiple platforms into local Docker daemon. Building for first platform only."
        PLATFORM_TO_BUILD=$(echo "$PLATFORMS" | cut -d',' -f1)
        KO_ARGS[1]="--platform=$PLATFORM_TO_BUILD"
    fi
    
    # Only set load if explicitly requested
    if [ "$LOAD" = true ]; then
        KO_ARGS+=("--image-refs=./ko-image-refs")
    fi
fi

# Add the path to build (current directory by default)
KO_ARGS+=("./cmd/server")

# Execute the build command
echo "Running: ko build ${KO_ARGS[*]}"
ko build "${KO_ARGS[@]}"

# Output the image information
echo "Image built successfully"
echo "Image: ${FULL_IMAGE_NAME}:${TAG}"
echo "Platforms: ${PLATFORMS}"

if [ "$PUSH" = false ] && [ "$LOAD" = true ]; then
    echo "To push the image, run:"
    echo "  docker push ${FULL_IMAGE_NAME}:${TAG}"
elif [ "$PUSH" = false ] && [ "$LOAD" = false ]; then
    echo "Image references saved to ./ko-image-refs"
    echo "To load the image, run:"
    echo "  docker load -i ./ko-image-refs"
fi