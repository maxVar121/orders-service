# Orders Service

A Go-based microservice that consumes order data from Kafka, stores it in PostgreSQL, caches it in memory, and provides an HTTP endpoint for retrieving orders by ID.

## Architecture

The service follows a simple event-driven architecture:

1. **Kafka Consumer**: listens to the `orders` topic for incoming order messages.
2. **PostgreSQL**: persists order data (order, delivery, payment, items) in a transactional manner.
3. **In-Memory Cache**: stores recently processed orders for fast access (with TTL of 10 minutes).
4. **Cache Persistence**: on shutdown, the cache is saved to a file (`order_cache.gob`). On startup, it is restored from the file or loaded from the database if the file is missing.
5. **HTTP Server**: provides a REST-like endpoint to retrieve order data by `order_uid`.
6. **Web Interface**: a simple HTML/JS page allows users to enter an order ID and view the result.

## Technologies

- **Language**: Go 1.24.4
- **Message Broker**: Apache Kafka (using `github.com/segmentio/kafka-go`)
- **Database**: PostgreSQL (using `github.com/jackc/pgx/v5/pgxpool`)
- **Caching**: In-memory thread-safe cache with expiration and file persistence
- **Web Server**: Standard `net/http` with HTML template
- **Build**: Multi-stage Docker build (static binary from scratch)
- **Orchestration**: Docker Compose

## Data Flow

1. Order is sent to Kafka as a JSON message.
2. Service consumes the message, validates it, and saves to PostgreSQL.
3. Order is added to in-memory cache.
4. On HTTP request to `/order/{order_uid}`:
   - Service checks cache first.
   - If not found, queries the database, returns result, and caches it.
5. On shutdown, the cache is saved to disk for recovery.

## Deployment

The service is containerized and deployed using Docker Compose.

### Requirements
- Docker
- Docker Compose

### Steps

1. Build and start all services:
   ```bash
   docker-compose up --build