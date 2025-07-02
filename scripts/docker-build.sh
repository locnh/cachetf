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
Usage: ./docker-build.sh [OPTIONS]
Build and optionally push Docker images for multiple platforms.

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
  ./docker-build.sh

  # Build and push multi-arch image
  ./docker-build.sh --push

  # Build with custom registry and tag
  ./docker-build.sh --registry=ghcr.io/username --tag=v1.0.0 --push

  # Build for specific platforms
  ./docker-build.sh --platforms=linux/arm64

  # Build without loading to local Docker
  ./docker-build.sh --no-load
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
    FULL_IMAGE_NAME="${DOCKER_REGISTRY}/${IMAGE_NAME}:${TAG}"
else
    FULL_IMAGE_NAME="${IMAGE_NAME}:${TAG}"
fi

echo "Building Docker image ${FULL_IMAGE_NAME} for platforms: ${PLATFORMS}"

# Ensure buildx is available
docker buildx version >/dev/null 2>&1 || {
    echo "Docker Buildx is required but not installed or not in PATH"
    exit 1
}

# Create a new builder instance if needed
BUILDER_NAME="cachetf-builder"
if ! docker buildx inspect "$BUILDER_NAME" >/dev/null 2>&1; then
    echo "Creating new builder: $BUILDER_NAME"
    docker buildx create --name "$BUILDER_NAME" --use
fi

# Determine build command based on whether we're pushing or loading
if [ "$PUSH" = true ]; then
    # For pushing, we can build all platforms
    BUILD_CMD=(
        docker buildx build
        --platform "$PLATFORMS"
        -t "$FULL_IMAGE_NAME"
        --push
        --build-arg VERSION="$TAG"
        --build-arg GIT_COMMIT="$(git rev-parse HEAD)"
        --build-arg BUILD_DATE="$(date -u +'%Y-%m-%dT%H:%M:%SZ')"
        .
    )
else
    # For local loading, we can only handle one platform at a time
    if [[ "$PLATFORMS" == *,* ]]; then
        echo "Warning: Cannot load multiple platforms into local Docker daemon. Building for first platform only."
        PLATFORM_TO_BUILD=$(echo "$PLATFORMS" | cut -d',' -f1)
    else
        PLATFORM_TO_BUILD="$PLATFORMS"
    fi

    BUILD_CMD=(
        docker buildx build
        --platform "$PLATFORM_TO_BUILD"
        -t "$FULL_IMAGE_NAME"
        --load
        --build-arg VERSION="$TAG"
        --build-arg GIT_COMMIT="$(git rev-parse HEAD)"
        --build-arg BUILD_DATE="$(date -u +'%Y-%m-%dT%H:%M:%SZ')"
        .
    )
fi

# Execute the build command
echo "Running: ${BUILD_CMD[*]}"
"${BUILD_CMD[@]}"

# Output the image information
echo "Image built successfully"
echo "Image: ${FULL_IMAGE_NAME}"
echo "Platforms: ${PLATFORMS}"

if [ "$PUSH" = false ]; then
    if [[ "$PLATFORMS" == *,* ]]; then
        echo "To push multi-arch image, run:"
        echo "  ./scripts/docker-build.sh --push"
    else
        echo "To push the image, run:"
        echo "  docker push ${FULL_IMAGE_NAME}"
    fi
fi