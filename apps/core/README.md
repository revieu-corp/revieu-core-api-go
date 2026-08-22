# Core Backend Service

A standard Go/Gin backend service following best practices and clean architecture principles.

## Project Structure

```
.
├── cmd/
│   └── app/
│       └── main.go        # Application entry point
├── internal/              # Private application code
│   ├── handler/           # HTTP handlers (Controller layer)
│   ├── service/           # Business logic layer
│   ├── repository/        # Data access layer (DAO/DB)
│   ├── model/             # Database models and domain entities
│   └── config/            # Configuration structures
├── pkg/                   # Public library code
│   ├── logger/            # Custom logger
│   └── utils/             # Utility functions
├── api/                   # API protocol definitions
│   ├── openapi/           # Swagger/OpenAPI specifications
├── configs/               # Reserved for non-configuration assets; runtime config uses environment variables
├── scripts/               # Build, install, and analysis scripts
├── build/                 # Packaging and CI
│   ├── package/           # Dockerfiles
│   └── ci/                # CI configuration files
├── deployments/           # Deployment configurations (K8s, Helm)
├── test/                  # External test data and integration tests
├── go.mod                 # Dependency management
├── go.sum
├── Makefile               # Common commands
└── README.md
```

## Getting Started

### Prerequisites

- Go 1.21 or higher
- Docker (optional, for containerization)

### Installation

1. Clone the repository
2. Install dependencies:
   ```bash
   make deps
   ```

### Running the Application

```bash
# Run directly
make run

# Or build and run
make build
./build/bin/core
```

### Development

```bash
# Format code
make fmt

# Run tests
make test

# Run tests with coverage
make test-coverage

# Run linter
make lint
```

`DB_AUTO_MIGRATE` defaults to `false` and should stay false in production. Runtime configuration is loaded from Cloudflare KV first, then process environment variables.

Use SQL migrations managed by Goose:

```bash
# install goose (one-time)
go install github.com/pressly/goose/v3/cmd/goose@latest

# create a new migration file template (no DB change yet)
make migrate-create name=add_coupon_scope_fields
# then edit apps/core/migrations/<new>.sql (-- +goose Up / -- +goose Down)

# apply all migrations
make migrate-up DB_DSN='postgres://postgres:postgres@localhost:5432/revieu?sslmode=disable'

# check status
make migrate-status DB_DSN='postgres://postgres:postgres@localhost:5432/revieu?sslmode=disable'
```

`make migrate-create` only generates the SQL migration file. The database schema changes when `make migrate-up` runs.

### Docker

```bash
# Build Docker image
make docker-build

# Run Docker container
make docker-run
```

## Configuration

The service does not read YAML, JSON, TOML, `.env`, or other configuration files. It loads configuration in this order:

```text
Cloudflare KV > process environment > code defaults
```

The Cloudflare KV adapter itself reads `CLOUDFLARE_ACCOUNT_ID`, `CLOUDFLARE_KV_NAMESPACE_ID`, `CLOUDFLARE_API_TOKEN`, optional `CLOUDFLARE_KV_PREFIX`, and optional `CLOUDFLARE_KV_TIMEOUT` from the process environment. Kubernetes should inject these through ConfigMap/Secret; local development should use `export`.

Application variables use names such as `SERVER_PORT`, `DB_HOST`, `DB_PASSWORD`, `JWT_SECRET`, and `GEMINI_API_KEY`.

The AI endpoint is protected by server-side, database-backed guardrails. The defaults are
5 requests per user per minute, 20 per client-IP per minute, 100 globally per minute,
50 requests per user per UTC day, and 500 per user per UTC month. Override them through
`GEMINI_USER_RATE_LIMIT_PER_MINUTE`, `GEMINI_IP_RATE_LIMIT_PER_MINUTE`,
`GEMINI_GLOBAL_RATE_LIMIT_PER_MINUTE`, `GEMINI_DAILY_QUOTA_PER_USER`, and
`GEMINI_MONTHLY_QUOTA_PER_USER`. Set an individual value to `0` only when that dimension
is intentionally disabled in an isolated environment.

## API Documentation

API documentation is available in the `api/openapi/` directory.

## License

[Your License Here]
