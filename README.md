# GMHelper Notify API

GMHelper Notify API is a standalone Go microservice for notification and email delivery operations in the GMHelper ecosystem.

## Purpose

This service provides the foundation for notification campaigns, email templates, direct user notifications, delivery tracking, automation rules, and application settings. The architecture keeps domain logic separate from HTTP, storage, and email provider implementations.

## Architecture Overview

- `cmd/notify-api`: application entrypoint
- `internal/api`: HTTP routing and REST API layer
- `internal/http/middleware`: middleware for logging, recovery, request IDs, and CORS
- `internal/app`: application-level services and abstractions
- `internal/domain`: domain models and repository interfaces
- `internal/infra`: infrastructure implementations for PostgreSQL, SMTP, and logging
- `Dockerfile`: container build specification
- `docker-compose.yml`: local development environment with PostgreSQL

## Local Development

1. Copy `.env.example` to `.env` and customize values.
2. Start services:

```bash
docker compose up --build
```

3. The API will be available at `http://localhost:8080`.

## Environment Variables

- `APP_ENV`: application environment (default: `development`)
- `HTTP_HOST`: HTTP host (default: `0.0.0.0`)
- `HTTP_PORT`: HTTP port (default: `8080`)
- `DATABASE_URL`: PostgreSQL connection string
- `SMTP_HOST`: SMTP server host
- `SMTP_PORT`: SMTP server port
- `SMTP_USERNAME`: SMTP username
- `SMTP_PASSWORD`: SMTP password
- `SMTP_FROM`: default sender email address
- `LOG_LEVEL`: structured log level (default: `info`)
- `ALLOWED_CORS_ORIGINS`: allowed CORS origins

## Docker

Build the API container:

```bash
docker build -t gmhelper-notify-api .
```

Use Docker Compose for local development:

```bash
docker compose up --build
```

## API Health Endpoints

- `GET /health`: returns service liveness
- `GET /ready`: returns readiness based on PostgreSQL availability

## Project Structure

- `cmd/notify-api`: main application launcher
- `internal/api`: HTTP routing and versioned endpoints
- `internal/api/handlers`: HTTP handler implementations
- `internal/http/middleware`: middleware definitions
- `internal/app/health`: readiness check service
- `internal/app/email`: email delivery abstraction
- `internal/app/background`: background processor skeleton
- `internal/domain`: domain entities and persistence contracts
- `internal/infra/postgres`: PostgreSQL database wrapper
- `internal/infra/smtp`: SMTP provider implementation
- `internal/infra/logger`: structured logging setup
- `internal/config`: environment-based configuration loader

## Testing

The initial architecture supports testable services through interfaces and mocks.

## Notes

The service is intentionally designed for future additions such as background workers, queueing, and alternate email providers without changing the HTTP or domain layers.
