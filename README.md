# MZL Payment API (Backend)

The robust backend service for the MZL Cross-Border Payment System. Built with **Golang**, this API handles secure authentication, multi-currency wallet management, real-time transaction processing, and third-party integrations.

## Tech Stack

- **Language:** Golang (1.21+)
- **Framework:** Chi Router (Lightweight & idiomatic)
- **Database:** PostgreSQL (Persistence)
- **ORM:** Prisma Client Go (Type-safe database access)
- **Caching & Queues:** Redis (Rate limiting, background jobs, idempotency)
- **Containerization:** Docker
- **Hosting:** Railway (Production)

## Key Features

- **Secure Authentication:** JWT-based stateless auth with HttpOnly cookie refresh tokens.
- **Wallet System:** Double-entry ledger system supporting multiple currencies (NGN, USD).
- **Concurrency Control:** Atomic database updates and strict constraints to prevent race conditions (double-spending).
- **Async Processing:** Redis-backed job queues for handling third-party webhooks and heavy tasks.
- **Security:**
  - Rate Limiting (Token Bucket algorithm via Redis).
  - Idempotency Keys (Prevent duplicate transactions).
  - Transaction PINs for sensitive operations.

## Environment Variables

Create a `.env` file in the root directory:

```bash
PORT=8080
DATABASE_URL="postgresql://user:password@localhost:5432/mzl_db?schema=public"
REDIS_URL="redis://localhost:6379"

# Security
JWT_SECRET="your-supery"


# External Services
PAYSTACK_SECRET_KEY="sk_test_..."
FRONT_END_URL="http://localhost:5173"
```
