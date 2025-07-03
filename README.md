# TF Registry Cache Proxy

A high-performance proxy server for Terraform/Tofu provider binaries with local or S3 backend caching capabilities. Built with Go, Gin, and Logrus.

## Features

- Network mirror caching of Terraform provider binaries
- Transparent proxy to upstream Terraform registry
- Configurable cache storage (local filesystem or S3)
- Structured logging with Logrus
- Environment-based configuration
- Graceful shutdown
- Support for multiple provider versions and platforms
- Support `metrics` endpoint for monitoring
- Support `DELETE` endpoint for deleting cached binaries

## Getting Started

### Prerequisites

- Go 1.19 or higher
- Docker
- Git
- Terraform (for testing)

### Docker

Run the Docker container:
```bash
docker run -d -p 8080:8080 -p 9090:9090 --name cachetf locnh/cachetf
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

   ### Basic Configuration
   ```env
   # Server configuration, default: 8080
   PORT=8080
   
   # Base path for API endpoints default: /providers
   URI_PREFIX=/providers
   
   # Storage type (local or s3, default: local)
   STORAGE_TYPE=local

   # S3 Configuration (required if STORAGE_TYPE=s3)
   # S3_BUCKET=your-bucket-name
   # S3_REGION=eu-central-1
   
   # For filesystem storage (default: ./cache)
   CACHE_DIR=./cache
   
   # Log level (debug, info, warn, error, default: info)
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
    ```hcl
    provider_installation {
        network_mirror {
            # keep trailing slash / and must be https
            url = "https://your-cache-server/providers/"
        }
    }
    ```

## Metrics

The application exposes metrics at `/metrics` endpoint. The metrics are exposed in Prometheus format.
```bash
# Metrics port, default: 9100
METRICS_PORT=9100
```

## API Endpoints

- `GET /health` - Health check endpoint
- `GET /providers/:registry/:namespace/:provider/index.json` - List available versions
- `GET /providers/:registry/:namespace/:provider/:version.json` - List available platforms
- `GET /providers/:registry/:namespace/:provider/terraform-provider-${provider}_${version}_${platform}_${arch}.zip` - Download provider binary
- `DELETE /providers/:registry/:namespace/:provider/:version` - Delete provider binary
- `DELETE /providers/:registry/:namespace/:provider` - Delete provider
- `DELETE /providers/:registry/:namespace` - Delete namespace
- `DELETE /providers/:registry` - Delete registry

## Configuration

### Environment Variables

| Variable            | Default           | Description                                                                 |
|---------------------|-------------------|-----------------------------------------------------------------------------|
| PORT                | 8080              | Port to run the server on                                                   |
| METRICS_PORT        | 9100              | Port to run the metrics server on                                           |
| URI_PREFIX          | /providers        | Base path for API endpoints                                                 |
| STORAGE_TYPE        | local             | Storage type: 'local' or 's3'                                               |
| CACHE_DIR           | ./cache           | Local directory for cached binaries (used when STORAGE_TYPE=local)          |
| LOG_LEVEL           | info              | Log level (debug, info, warn, error)                                        |
| S3_BUCKET           | -                 | S3 bucket name (required for S3 storage)                                    |
| S3_REGION           | eu-central-1      | AWS region for S3 storage                                                   |

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

## S3 Storage Configuration

To use S3 as the storage backend, set the following environment variables:

1. Set `STORAGE_TYPE=s3`
2. Configure your AWS credentials and bucket:
   ```env
   STORAGE_TYPE=s3
   S3_BUCKET=your-bucket-name
   S3_REGION=eu-central-1
   ```

### S3 IAM Permissions

The following IAM permissions are required for the S3 bucket:

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "s3:GetObject",
                "s3:PutObject",
                "s3:ListBucket"
            ],
            "Resource": [
                "arn:aws:s3:::your-bucket-name",
                "arn:aws:s3:::your-bucket-name/*"
            ]
        }
    ]
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
