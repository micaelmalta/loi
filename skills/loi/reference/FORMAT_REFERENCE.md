# LOI Entry Format Reference

## Field Guide

Every LOI entry (`# filename.ext`) should include only applicable fields from:

| Field | When to Include | Example |
|-------|-----------------|---------|
| **DOES** | Always required | "Validates JWT tokens, extracts claims, stores in context" |
| **SYMBOLS** | Files with exported functions/types | `GetUser(ctx, id string) → (User, error)` |
| **TYPE** | Files defining struct/class types | `User { ID, Name, Email, CreatedAt }` |
| **INTERFACE** | Files defining interface contracts | `Reader { Read() ([]byte, error) }` |
| **ROUTES** | HTTP/gRPC handlers only | `GET /users/:id → GetUser` |
| **CONFIG** | Files using environment variables | `DB_HOST, DB_PORT, API_KEY` |
| **TABLE** | Files with database operations | `users (SELECT, UPDATE, DELETE)` |
| **CONSUMERS** | Event/message subscribers | `topic.UserCreated, topic.UserDeleted` |
| **EMITS** | Event/message publishers | `topic.OrderPlaced, topic.PaymentProcessed` |
| **DEPENDS** | Files that import key internal modules | `auth/jwt.go, persist/users.go` |
| **PROPS** | React/frontend component inputs | `{ user: User, onSuccess?: () => void }` |
| **HOOKS** | React custom hooks used or exported | `useAuth(), useForm(initialValues)` |
| **PATTERNS** | Files implementing a cross-cutting strategy | `exponential-backoff`, `circuit-breaker`, `saga` |
| **USE WHEN** | Decision-critical files (multiple options exist) | "Choose when need sync email validation (not async)" |

**Omit fields with no content. No empty keys.**

### Field Rules

- **DOES**: Always present. Specific action/outcome, never generic. If a file implements a behavioral pattern (retry, backoff, circuit breaker), name the pattern and its key parameters.
- **SYMBOLS**: Only exported/public functions and types. Include full signatures with params and return types. Never wrap in backticks — use plain text (e.g., `GetUser(ctx, id string) → (User, error)` not `` `GetUser(ctx, id string) → (User, error)` ``).
- **DEPENDS**: Only list internal imports that represent meaningful cross-domain coupling. Skip standard library, same-package, and trivial utility imports. How to identify per language:
  - **Go**: `import` paths crossing `internal/` subdirectories (e.g., `internal/scan` importing `internal/audit`)
  - **Python**: relative or absolute imports from other top-level packages (e.g., `from guard.integrity_check import ...`)
  - **TypeScript/JS**: imports from other `src/` subdirectories or packages (e.g., `import { useApi } from '@/composables/useApi'`)
  - Format: use the shortest unambiguous internal path relative to the source root (e.g., `internal/audit`, `internal/scan`, `persist/users.go`). Never prefix with the module name or repo name — use `internal/audit` not `myapp/internal/audit`.
- **EMITS / CONSUMERS**: Use together to trace event flows across rooms. If a file both publishes and subscribes, include both fields. EMITS also covers: callback invocations that notify other subsystems, webhook dispatch, channel sends, and event bus publishes.
- **PROPS / HOOKS**: Use for React/Vue/frontend components. PROPS lists the component's input contract (props/attributes). HOOKS lists custom hooks (React `useX()`) or composables (Vue `useX()`) that are either exported from or used within the file.
- **USE WHEN**: Only add when multiple files could serve a similar purpose and the reader needs guidance to pick the right one.

### Entry Ordering

Entries within a room file are **alphabetized by filename**. This makes scanning predictable and diffs cleaner.

```
# auth/jwt.go
...

# auth/oauth.go
...

# auth/session.go
...
```

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
DEPENDS: persist/users.go, auth/jwt.go
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
DEPENDS: persist/users.go, auth/jwt.go
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
EMITS: topic.PaymentProcessed, topic.RefundIssued
DEPENDS: config/config.go
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
DEPENDS: integrations/email.go, persist/sessions.go
```

```
# publishers/order_events.go
DOES: Publishes order lifecycle events after successful DB commits
SYMBOLS:
- PublishOrderPlaced(ctx, order Order) → error
- PublishOrderShipped(ctx, orderID string, tracking string) → error
EMITS: kafka.topic.OrderPlaced, kafka.topic.OrderShipped
DEPENDS: persist/orders.go
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
DEPENDS: persist/users.go, integrations/email.go
CONFIG: OTP_LENGTH, OTP_TTL_SECONDS, SMTP_HOST, RETRY_MAX_ATTEMPTS
PATTERNS: exponential-backoff
```

### Frontend Components

```
# UserForm.tsx
DOES: Profile update form (name, email, avatar) with validation and submit; publishes onSubmit event
SYMBOLS:
- UserForm(props: UserFormProps) → JSX.Element
- useUserForm(initialValues) → { values, errors, touched, setFieldValue, handleSubmit }
TYPE: UserFormProps { user: User, onSuccess?: () => void, disabled?: boolean }
PROPS: { user: User, onSuccess?: () => void, disabled?: boolean }
HOOKS: useUserForm
```

```
# AuthProvider.tsx
DOES: React context provider for auth state; wraps app tree, exposes login/logout/currentUser
SYMBOLS:
- AuthProvider(props: { children: ReactNode }) → JSX.Element
- useAuth() → { user: User | null, login, logout, isLoading }
HOOKS: useAuth
DEPENDS: api/client.ts, auth/token.ts
USE WHEN: Any component needs current user state or login/logout actions
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
- **Rule:** If a file implements a behavioral pattern (retry, backoff, circuit breaker, rate limit, saga, pub/sub), name the pattern and its key parameters in DOES.

**SYMBOLS incomplete:**
- Bad: `GetUser`
- Good: `GetUser(ctx context.Context, id string) → (User, error)`

**SYMBOLS lists private/unexported functions:**
- Bad: `parseToken(raw string) → (Claims, error)` (unexported helper)
- Good: omit unexported functions — only list public API surface

**ROUTES missing HTTP method:**
- Bad: `/users/:id → GetUser`
- Good: `GET /users/:id → GetUser`

**TYPE lists every field:**
- Bad: `User { ID, Name, Email, Bio, AvatarURL, Phone, Address, City, State, Zip, Country, CreatedAt, UpdatedAt, DeletedAt, IsActive, ... }`
- Good: `User { ID, Name, Email, CreatedAt }` — list key fields only; omit boilerplate (timestamps, soft-delete flags) unless they drive business logic

**Empty fields** (don't include):
- Bad: `TYPE: None` / `INTERFACE: None`
- Good: omit the line entirely

**Missing PATTERNS field:**
- Bad: (omitted when file has exponential backoff, circuit breaker, etc.)
- Good: `PATTERNS: exponential-backoff, at-least-once-delivery`
- **Rule:** If you can name the design pattern or resilience strategy, add PATTERNS.

**DEPENDS on everything:**
- Bad: `DEPENDS: utils/logger.go, utils/errors.go, utils/strings.go` (trivial utilities)
- Good: `DEPENDS: persist/users.go, auth/jwt.go` — only meaningful cross-domain coupling

**DEPENDS with module prefix:**
- Bad: `DEPENDS: myapp/internal/audit, dep-registry/internal/scan`
- Good: `DEPENDS: internal/audit, internal/scan` — shortest unambiguous path from source root

**SYMBOLS with backticks:**
- Bad: `` `GetUser(ctx, id string) → (User, error)` ``
- Good: `GetUser(ctx, id string) → (User, error)` — plain text, no markdown formatting

**Splitting by alphabet:**
- Bad: `models_a_to_m.md` / `models_n_to_z.md`
- Good: `models_core.md` / `models_orders.md` / `models_compliance.md`

---

## Checklist for New Domain Files

- [ ] Each entry has DOES (always required)
- [ ] SYMBOLS include full signatures with params and return types
- [ ] SYMBOLS only list exported/public functions
- [ ] No TYPE/INTERFACE/ROUTES/CONFIG/TABLE unless applicable
- [ ] DEPENDS only lists meaningful cross-domain imports
- [ ] EMITS and CONSUMERS used together to trace event flows
- [ ] PATTERNS included for any behavioral strategy (retry, backoff, etc.)
- [ ] No prose, no empty keys
- [ ] Entries alphabetized by filename within room
- [ ] File format: `# filename.ext` (not just `# filename`)
- [ ] Room file stays under ~150 entries
- [ ] Building `_root.md` has TASK → LOAD table and Rooms listing
- [ ] Campus `_root.md` routes to all buildings
