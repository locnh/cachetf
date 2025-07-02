# Terraform Registry Cache Proxy

A high-performance proxy server for Terraform provider binaries with local caching capabilities. Built with Go, Gin, and Logrus.

## Features

- Network mirror caching of Terraform provider binaries
- Transparent proxy to upstream Terraform registry
- Configurable cache directory
- Structured logging with Logrus
- Environment-based configuration
- Graceful shutdown
- Support for multiple provider versions and platforms

## Getting Started

### Prerequisites

- Go 1.19 or higher
- Docker
- Git
- Terraform (for testing)

### Docker

Run the Docker container:
```bash
docker run -d -p 8080:8080 --name cachetf locnh/cachetf
```

### Docker compose (recommended)

```bash
docker compose up -d
```

### Build from source

1. Clone the repository:
   ```bash
   git clone <repository-url>
   cd <project-directory>
   ```

2. Install dependencies:
   ```bash
   go mod tidy
   ```

3. Create a `.env` file in the root directory and configure your environment variables:
   ```env
   # Server configuration
   PORT=8080
   
   # Base path for API endpoints (default: /providers)
   URI_PREFIX=/providers
   
   # Cache directory (default: ./cache)
   CACHE_DIR=./cache
   
   # Log level (debug, info, warn, error)
   LOG_LEVEL=info
   ```

4. Build and run the application:
   ```bash
   # Using default configuration
   go run cmd/server/main.go
   
   # Or with custom cache directory
   CACHE_DIR=./my-cache go run cmd/server/main.go
   ```

5. Configure Terraform to use the [network mirror](https://developer.hashicorp.com/terraform/internals/provider-network-mirror-protocol#protocol-base-url) cache

## Project Structure

```
.
├── cmd/
│   └── server/          # Main application entry point
├── internal/
│   ├── config/         # Configuration loading and validation
│   ├── handler/        # HTTP request handlers
│   └── routes/         # Route definitions and middleware
├── pkg/
│   └── logger/         # Logging utilities
├── .env.example       # Example environment variables
├── .gitignore         # Git ignore file
├── go.mod             # Go module definition
├── go.sum             # Go module checksums
└── README.md          # This file
```

## API Endpoints

- `GET /health` - Health check endpoint
- `GET /providers/:registry/:namespace/:provider/index.json` - List available versions
- `GET /providers/:registry/:namespace/:provider/:version.json` - List available platforms
- `GET /providers/:registry/:namespace/:provider/terraform-provider-${provider}_${version}_${platform}_${arch}.zip` - Download provider binary

## Configuration

### Environment Variables

| Variable   | Default    | Description                                 |
| ---------- | ---------- | ------------------------------------------- |
| PORT       | 8080       | Port to run the server on                   |
| URI_PREFIX | /providers | Base path for API endpoints                 |
| CACHE_DIR  | ./cache    | Directory to store cached provider binaries |
| LOG_LEVEL  | info       | Log level (debug, info, warn, error)        |

### Logging

The application uses Logrus for structured logging. Logs are output in JSON format. Set `LOG_LEVEL=debug` for more verbose logging.

Example log output:
```json
{
  "level": "info",
  "msg": "Successfully downloaded and verified provider binary",
  "cache_path": "./cache/registry.terraform.io/hashicorp/random/3.7.2/terraform-provider-random_3.7.2_darwin_arm64.zip",
  "sha256": "1e86bcd7ebec85ba336b423ba1db046aeaa3c0e5f921039b3f1a6fc2f978feab",
  "time": "2025-07-02T02:14:59+02:00"
}
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Commit your changes
4. Push to the branch
5. Create a new Pull Request

## License

This project is licensed under the MIT License.
