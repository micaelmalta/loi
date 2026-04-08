# LOI Entry Format Reference

## Field Guide

Every LOI entry (`# filename.ext`) should include only applicable fields from:

| Field | When to Include | Example |
|-------|-----------------|---------|
| **DOES** | Always required | "Validates JWT tokens, extracts claims, stores in context" |
| **SYMBOLS** | Files with exported functions/types | `GetUser(ctx, id string) → (User, error)` |
| **TYPE** | Files defining struct types | `User { ID, Name, Email, CreatedAt }` |
| **INTERFACE** | Files defining interface contracts | `Reader { Read() ([]byte, error) }` |
| **ROUTES** | HTTP handlers only | `GET /users/:id → GetUser` |
| **CONFIG** | Files using environment variables | `DB_HOST, DB_PORT, API_KEY` |
| **TABLE** | Files with database operations | `users (SELECT, UPDATE, DELETE)` |
| **CONSUMERS** | Event/message subscribers | `topic.UserCreated, topic.UserDeleted` |
| **USE WHEN** | Decision-critical files | "Choose when need user email validation (not async)" |

| **PATTERNS** | Files implementing a cross-cutting strategy | `exponential-backoff`, `circuit-breaker`, `saga` |

**Omit fields with no content. No empty keys.**

---

## Examples

### infra/ — Startup & Config

```
# config/config.go
DOES: Loads config from env vars, validates required fields, provides type-safe access
SYMBOLS:
- Load() → (Config, error)
- (c Config) Validate() → error
TYPE: Config { DBHost, DBPort, APIKey, LogLevel, MaxConnections int }
CONFIG: DB_HOST, DB_PORT, DB_USER, DB_PASS, API_KEY, LOG_LEVEL, MAX_CONNECTIONS
```

```
# migrations/20240101_0000_create_users_table.sql
DOES: Creates users table with email uniqueness constraint, indexed created_at for range queries
TABLE: users (id BIGINT PRIMARY KEY, email VARCHAR UNIQUE, name, created_at INDEXED)
```

### identity/ — Auth & Permissions

```
# auth/jwt.go
DOES: Validates JWT tokens, extracts claims, stores user context for downstream handlers
SYMBOLS:
- ValidateToken(tokenString string) → (Claims, error)
- ExtractClaims(r *http.Request) → (Claims, error)
- AuthMiddleware(next http.Handler) → http.Handler
CONFIG: JWT_SECRET, JWT_EXPIRY_HOURS
PATTERNS: middleware-chain
USE WHEN: Protecting routes that need authenticated user context
```

```
# auth/oauth.go
DOES: OAuth2 flow for Google/GitHub login; exchanges code for token, creates or links user account
SYMBOLS:
- BeginOAuth(provider string) → (redirectURL string, state string, error)
- CompleteOAuth(ctx, provider, code, state string) → (User, error)
CONFIG: GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET, GITHUB_CLIENT_ID, GITHUB_CLIENT_SECRET
PATTERNS: oauth2-authorization-code
```

### api/ — HTTP Handlers

```
# handlers/user.go
DOES: HTTP handlers for user CRUD with JSON parsing and auth middleware
SYMBOLS:
- CreateUser(w http.ResponseWriter, r *http.Request, ps httprouter.Params) → error
- GetUser(w http.ResponseWriter, r *http.Request, ps httprouter.Params) → error
- UpdateUser(w http.ResponseWriter, r *http.Request, ps httprouter.Params) → error
- DeleteUser(w http.ResponseWriter, r *http.Request, ps httprouter.Params) → error
ROUTES:
- POST /users → CreateUser
- GET /users/:id → GetUser
- PATCH /users/:id → UpdateUser
- DELETE /users/:id → DeleteUser
```

### data/ — Persistence

```
# persist/users.go
DOES: Database queries for users table (insert, select, update, delete) with transaction support
SYMBOLS:
- CreateUser(ctx, user User) → error
- GetUser(ctx, id string) → (User, error)
- UpdateUser(ctx, user User) → error
- DeleteUser(ctx, id string) → error
TABLE: users, user_settings
```

### integrations/ — External APIs

```
# integrations/stripe.go
DOES: Stripe payment wrapper; creates customers, charges, refunds with exponential backoff retry (500ms base, 3 attempts, 5s cap) and Stripe error code mapping
SYMBOLS:
- CreateCustomer(ctx, user User) → (string, error)
- ChargeCard(ctx, customerID, amount, description string) → (string, error)
- RefundCharge(ctx, chargeID string) → error
CONFIG: STRIPE_API_KEY, STRIPE_WEBHOOK_SECRET
PATTERNS: exponential-backoff, error-mapping
USE WHEN: Processing payments, customer lifecycle, refunds
```

### workers/ — Background Jobs & Events

```
# consumers/user_events.go
DOES: Consumes UserCreated/UserDeleted events; triggers welcome email and session cleanup
SYMBOLS:
- HandleUserCreated(ctx context.Context, event events.UserCreated) → error
- HandleUserDeleted(ctx context.Context, event events.UserDeleted) → error
CONSUMERS: kafka.topic.UserCreated, kafka.topic.UserDeleted
```

### Business Domain — Core Logic

```
# user_validation.go
DOES: Validates user email format and uniqueness; sends OTP with exponential backoff retries
SYMBOLS:
- ValidateEmail(ctx, email string) → error
- IsEmailUnique(ctx, email string) → (bool, error)
- SendOTP(ctx, email string) → (string, error)
- VerifyOTP(ctx, email, otp string, timeout time.Duration) → error
CONFIG: OTP_LENGTH, OTP_TTL_SECONDS, SMTP_HOST, RETRY_MAX_ATTEMPTS
```

### Frontend Components

```
# UserForm.tsx
DOES: Profile update form (name, email, avatar) with validation and submit; publishes onSubmit event
SYMBOLS:
- UserForm(props: UserFormProps) → JSX.Element
- useUserForm(initialValues) → { values, errors, touched, setFieldValue, handleSubmit }
TYPE: UserFormProps { user: User, onSuccess?: () => void, disabled?: boolean }
```

---

## Anti-Patterns

**DOES too generic:**
- Bad: `handles user data`
- Good: `Creates new users with email validation; publishes UserCreated event; handles duplicate email error`

**DOES hides the _how_ (behavioral strategy):**
- Bad: `Starts replication with retry logic for connection errors`
- Good: `Starts replication with exponential backoff retry (2s initial, 2x multiplier, 30s cap) on connection and duplicate server_id errors`
- Bad: `Processes events with error handling`
- Good: `Processes CDC events in batches; stores offsets with exponential backoff retry (100ms base, 3 retries, 2s cap); adds idempotency keys`
- **Rule:** If a file implements a behavioral pattern (retry, backoff, circuit breaker, rate limit, saga, pub/sub), name the pattern and its key parameters in DOES. These are the keywords users search for.

**SYMBOLS incomplete:**
- Bad: `GetUser`
- Good: `GetUser(ctx context.Context, id string) → (User, error)`

**Empty fields** (don't include):
- Bad: `TYPE: None` / `INTERFACE: None`
- Good: omit the line entirely

**Missing PATTERNS field:**
- Bad: (omitted when file has exponential backoff, circuit breaker, etc.)
- Good: `PATTERNS: exponential-backoff, at-least-once-delivery`
- **Rule:** If you can name the design pattern or resilience strategy, add PATTERNS.

**Splitting by alphabet:**
- Bad: `models_a_to_m.md` / `models_n_to_z.md`
- Good: `models_core.md` / `models_orders.md` / `models_compliance.md`

---

## Checklist for New Domain Files

- [ ] Each entry has DOES (always required)
- [ ] SYMBOLS include full signatures when applicable
- [ ] No TYPE/INTERFACE/ROUTES/CONFIG/TABLE unless applicable
- [ ] No prose, no empty keys
- [ ] Entries alphabetized by filename within domain
- [ ] File format: `# filename.ext` (not just `# filename`)
- [ ] Room file stays under ~150 entries
- [ ] Building `_root.md` has TASK → LOAD table and Rooms listing
- [ ] Campus `_root.md` routes to all buildings
