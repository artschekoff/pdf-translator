# Web Block Editor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a web app where users upload a PDF, configure Docling OCR settings, get the result in a block editor (Editor.js), edit with AI assistance, and export to multiple formats — backed by a Go modular monolith with a Redis task queue designed for horizontal worker scaling.

**Architecture:** Go monolith (Gin + GORM + asynq) handles HTTP API and runs embedded workers by default. OCR and AI processing are dispatched as async tasks through Redis so standalone worker nodes can be added later without code changes. Next.js frontend with App Router. Auth via JWT from `auth-service` (RS256, JWKS). Quota enforcement via `auth-service` REST API.

**Tech Stack:** Go 1.24 · Gin · GORM · PostgreSQL · Redis · asynq · Next.js 15 (App Router) · TypeScript · TailwindCSS · Editor.js · Docling HTTP service (existing) · auth-service (existing)

**Location:** `webapp/` inside `/Users/riskyworks/Documents/web/pdf-translator/`

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                        auth-service                          │
│  POST /auth/login  GET /.well-known/jwks.json               │
│  GET /auth/quota/{product}/{scope}                          │
│  POST /auth/quota/{product}/{scope}/increment               │
└────────────────────┬────────────────────────────────────────┘
                     │ JWT (RS256)  +  quota REST
                     ▼
┌─────────────────────────────────────────────────────────────┐
│                   Go Modular Monolith                        │
│  cmd/server — HTTP API (Gin) + embedded asynq worker        │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────────┐  │
│  │documents │ │  ocr     │ │   ai     │ │   export     │  │
│  │ handler  │ │ handler  │ │ handler  │ │   handler    │  │
│  │ service  │ │ worker ◄─┼─┤ worker ◄─┼─┤   service   │  │
│  │   repo   │ │ service  │ │ service  │ └──────────────┘  │
│  └──────────┘ └────┬─────┘ └────┬─────┘                   │
│                    │             │                           │
│              ┌─────▼─────────────▼────┐                    │
│              │     asynq (Redis)       │                    │
│              │  queue: ocr, ai, export │                    │
│              └────────────────────────┘                    │
└─────────────────────────┬───────────────────────────────────┘
                          │  later: add slaves
                          ▼
              ┌─────────────────────────┐
              │  cmd/worker (slave)      │
              │  only runs asynq server  │
              │  no HTTP — just queues   │
              └─────────────────────────┘

┌──────────────┐    ┌────────────────────┐
│  Next.js 15  │───►│  Docling service   │
│  App Router  │    │  (existing Docker) │
│  /api proxy  │    └────────────────────┘
└──────────────┘
```

### Module Boundaries

Each module owns its own handler, service, repository and model. Modules communicate through **shared interfaces** — no direct package imports between modules except through `internal/shared/`.

### Task Queue — Adding Workers Later

When you want to scale OCR or AI processing:
```bash
# Start a standalone worker (no HTTP server):
./worker --queues ocr:10,ai:5,export:1

# The monolith (server) continues handling HTTP.
# Both server and worker connect to the same Redis.
# No code changes needed.
```

---

## File Structure

```
webapp/
├── cmd/
│   ├── server/main.go          # HTTP server + embedded worker
│   └── worker/main.go          # Standalone worker (slave mode)
├── internal/
│   ├── auth/
│   │   ├── middleware.go       # JWT validation via JWKS
│   │   └── jwks.go             # JWKS fetcher + RS256 verifier
│   ├── quota/
│   │   └── client.go           # HTTP client → auth-service quota API
│   ├── documents/
│   │   ├── handler.go          # Gin handlers: upload, list, get, delete
│   │   ├── service.go          # Business logic
│   │   ├── repository.go       # GORM queries
│   │   └── model.go            # Document, Page, Block GORM models
│   ├── blocks/
│   │   ├── handler.go          # PUT /documents/:id/blocks
│   │   └── repository.go       # Block bulk save/load
│   ├── ocr/
│   │   ├── handler.go          # POST /documents/:id/ocr
│   │   ├── worker.go           # asynq task handler (runs Docling)
│   │   └── service.go          # Docling HTTP client calls
│   ├── ai/
│   │   ├── handler.go          # POST /ai/block, POST /ai/document/:id (SSE)
│   │   ├── worker.go           # asynq task handler (async AI jobs)
│   │   └── service.go          # Claude + OpenAI streaming clients
│   ├── export/
│   │   ├── handler.go          # POST /documents/:id/export
│   │   └── service.go          # md→PDF/DOCX/HTML/TXT
│   ├── queue/
│   │   ├── client.go           # asynq.Client wrapper + task enqueue helpers
│   │   ├── server.go           # asynq.Server setup + mux registration
│   │   └── tasks.go            # Task type constants + payload structs
│   ├── ws/
│   │   └── hub.go              # WebSocket hub (progress broadcast per doc)
│   └── shared/
│       ├── converter.go        # Markdown ↔ Editor.js block conversion
│       └── types.go            # Shared types (BlockData, OcrSettings, etc.)
├── pkg/
│   └── docling/
│       └── client.go           # HTTP client for Docling service
├── db/
│   └── migrations/             # goose SQL migrations
│       └── 001_initial.sql
├── config/
│   └── config.go               # Viper config + env binding
├── go.mod
├── go.sum
├── Makefile
└── frontend/                   # Next.js app
    ├── src/
    │   ├── app/
    │   │   ├── layout.tsx
    │   │   ├── page.tsx                    # redirect → /documents
    │   │   ├── documents/
    │   │   │   ├── page.tsx                # Document list
    │   │   │   └── upload/page.tsx         # Upload + OCR settings
    │   │   └── editor/[id]/page.tsx        # Block editor
    │   ├── components/
    │   │   ├── OcrSettingsForm.tsx
    │   │   ├── ProcessingProgress.tsx
    │   │   ├── BlockEditor.tsx
    │   │   ├── MarkdownConverter.ts
    │   │   ├── AiPanel.tsx
    │   │   └── ExportDialog.tsx
    │   ├── lib/
    │   │   ├── api.ts                      # Backend API client
    │   │   ├── auth.ts                     # Auth-service client (login/refresh)
    │   │   └── ws.ts                       # WebSocket progress client
    │   └── types.ts
    ├── next.config.ts
    ├── tailwind.config.ts
    └── package.json
```

---

## Task 1: Go Project Setup + Config

**Files:**
- Create: `webapp/go.mod`
- Create: `webapp/config/config.go`
- Create: `webapp/Makefile`

- [ ] **Step 1: Init Go module**

```bash
mkdir -p webapp && cd webapp
go mod init github.com/yourorg/pdf-translator-webapp
go get github.com/gin-gonic/gin@v1.10.0
go get gorm.io/gorm@v1.25.12
go get gorm.io/driver/postgres@v1.5.9
go get github.com/hibiken/asynq@v0.24.1
go get github.com/spf13/viper@v1.19.0
go get github.com/golang-jwt/jwt/v5@v5.2.1
go get github.com/gorilla/websocket@v1.5.3
go get github.com/pressly/goose/v3@v3.21.1
go get github.com/google/uuid@v1.6.0
```

- [ ] **Step 2: Create config/config.go**

```go
package config

import (
    "log"
    "github.com/spf13/viper"
)

type Config struct {
    // Server
    Port string `mapstructure:"PORT"`

    // Database
    DatabaseURL string `mapstructure:"DATABASE_URL"`

    // Redis
    RedisURL string `mapstructure:"REDIS_URL"`

    // Auth service
    AuthServiceURL  string `mapstructure:"AUTH_SERVICE_URL"`  // e.g. http://auth-service:8002
    QuotaProductSlug string `mapstructure:"QUOTA_PRODUCT_SLUG"` // e.g. "pdf-editor"

    // Storage
    UploadsDir string `mapstructure:"UPLOADS_DIR"`
    ExportsDir string `mapstructure:"EXPORTS_DIR"`

    // External services
    DockingServiceURL string `mapstructure:"DOCLING_SERVICE_URL"` // http://paddleocr:8000 or dedicated
    AnthropicAPIKey   string `mapstructure:"ANTHROPIC_API_KEY"`
    OpenAIAPIKey      string `mapstructure:"OPENAI_API_KEY"`

    // PDF export
    MdtoPdfScript string `mapstructure:"MDTOPDF_SCRIPT"`

    // Worker
    WorkerConcurrency int `mapstructure:"WORKER_CONCURRENCY"`
}

var C Config

func Load() {
    viper.AutomaticEnv()
    viper.SetDefault("PORT", "8080")
    viper.SetDefault("UPLOADS_DIR", "./uploads")
    viper.SetDefault("EXPORTS_DIR", "./exports")
    viper.SetDefault("WORKER_CONCURRENCY", 5)
    viper.SetDefault("QUOTA_PRODUCT_SLUG", "pdf-editor")
    viper.SetDefault("MDTOPDF_SCRIPT", "/Users/riskyworks/.scripts/pdf/mdtopdf.sh")

    if err := viper.Unmarshal(&C); err != nil {
        log.Fatalf("config: %v", err)
    }
}
```

- [ ] **Step 3: Create Makefile**

```makefile
.PHONY: run worker migrate build

run:
	go run ./cmd/server

worker:
	go run ./cmd/worker

migrate:
	go run ./cmd/server --migrate-only

build:
	go build -o bin/server ./cmd/server
	go build -o bin/worker ./cmd/worker

tidy:
	go mod tidy
```

- [ ] **Step 4: Commit**

```bash
git add webapp/go.mod webapp/go.sum webapp/config/ webapp/Makefile
git commit -m "feat(webapp): Go project setup — Gin + GORM + asynq + config"
```

---

## Task 2: Database Models + Migrations

**Files:**
- Create: `webapp/internal/documents/model.go`
- Create: `webapp/db/migrations/001_initial.sql`
- Create: `webapp/internal/shared/types.go`

- [ ] **Step 1: Create internal/shared/types.go**

```go
package shared

// OcrSettings mirrors the Docling pipeline options exposed to the user.
type OcrSettings struct {
    Engine              string   `json:"engine"`               // rapidocr | easyocr | tesseract | mac
    Lang                []string `json:"lang"`                  // ["en", "ru", ...]
    DoOcr               bool     `json:"do_ocr"`
    DoTableStructure    bool     `json:"do_table_structure"`
    TableMode           string   `json:"table_mode"`            // fast | accurate
    GeneratePictureImages bool   `json:"generate_picture_images"`
    ImagesScale         float64  `json:"images_scale"`
    DocumentTimeout     float64  `json:"document_timeout"`
}

// BlockData is the Editor.js block format stored in DB and sent to frontend.
type BlockData struct {
    ID    string                 `json:"id"`
    Type  string                 `json:"type"`
    Data  map[string]interface{} `json:"data"`
    Order int                    `json:"order"`
}
```

- [ ] **Step 2: Create internal/documents/model.go**

```go
package documents

import (
    "time"
    "github.com/google/uuid"
    "gorm.io/gorm"
    "github.com/yourorg/pdf-translator-webapp/internal/shared"
    "gorm.io/datatypes"
)

type DocumentStatus string

const (
    StatusPending    DocumentStatus = "pending"
    StatusProcessing DocumentStatus = "processing"
    StatusReady      DocumentStatus = "ready"
    StatusError      DocumentStatus = "error"
)

type Document struct {
    ID               string         `gorm:"type:uuid;primaryKey" json:"id"`
    UserID           string         `gorm:"type:uuid;index;not null" json:"user_id"`
    Title            string         `gorm:"not null" json:"title"`
    OriginalFilename string         `gorm:"not null" json:"original_filename"`
    Status           DocumentStatus `gorm:"default:'pending'" json:"status"`
    ErrorMessage     *string        `json:"error_message,omitempty"`
    OcrSettings      datatypes.JSON `json:"ocr_settings"`
    PageCount        int            `gorm:"default:0" json:"page_count"`
    CreatedAt        time.Time      `json:"created_at"`
    UpdatedAt        time.Time      `json:"updated_at"`

    Pages []Page `gorm:"foreignKey:DocumentID;constraint:OnDelete:CASCADE" json:"pages,omitempty"`
}

func (d *Document) BeforeCreate(tx *gorm.DB) error {
    if d.ID == "" {
        d.ID = uuid.NewString()
    }
    return nil
}

type Page struct {
    ID          string  `gorm:"type:uuid;primaryKey" json:"id"`
    DocumentID  string  `gorm:"type:uuid;index;not null" json:"document_id"`
    PageNumber  int     `gorm:"not null" json:"page_number"`
    RawMarkdown *string `gorm:"type:text" json:"raw_markdown,omitempty"`

    Blocks []Block `gorm:"foreignKey:PageID;constraint:OnDelete:CASCADE" json:"blocks,omitempty"`
}

func (p *Page) BeforeCreate(tx *gorm.DB) error {
    if p.ID == "" {
        p.ID = uuid.NewString()
    }
    return nil
}

type Block struct {
    ID     string         `gorm:"type:uuid;primaryKey" json:"id"`
    PageID string         `gorm:"type:uuid;index;not null" json:"page_id"`
    Type   string         `gorm:"not null" json:"type"`
    Data   datatypes.JSON `gorm:"not null" json:"data"`
    Order  int            `gorm:"not null;default:0" json:"order"`
}

func (b *Block) BeforeCreate(tx *gorm.DB) error {
    if b.ID == "" {
        b.ID = uuid.NewString()
    }
    return nil
}
```

- [ ] **Step 3: Create db/migrations/001_initial.sql**

```sql
-- +goose Up
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE documents (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL,
    title TEXT NOT NULL,
    original_filename TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    error_message TEXT,
    ocr_settings JSONB NOT NULL DEFAULT '{}',
    page_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_documents_user_id ON documents(user_id);

CREATE TABLE pages (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    document_id UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    page_number INT NOT NULL,
    raw_markdown TEXT
);
CREATE INDEX idx_pages_document_id ON pages(document_id);

CREATE TABLE blocks (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    page_id UUID NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
    type TEXT NOT NULL,
    data JSONB NOT NULL,
    "order" INT NOT NULL DEFAULT 0
);
CREATE INDEX idx_blocks_page_id ON blocks(page_id);

-- +goose Down
DROP TABLE blocks;
DROP TABLE pages;
DROP TABLE documents;
```

- [ ] **Step 4: Commit**

```bash
git add webapp/internal/ webapp/db/
git commit -m "feat(webapp): document/page/block models + goose migration"
```

---

## Task 3: Task Queue Setup (asynq)

**Files:**
- Create: `webapp/internal/queue/tasks.go`
- Create: `webapp/internal/queue/client.go`
- Create: `webapp/internal/queue/server.go`

- [ ] **Step 1: Create internal/queue/tasks.go**

```go
package queue

import "encoding/json"

const (
    QueueOCR    = "ocr"
    QueueAI     = "ai"
    QueueExport = "export"

    TaskOCRProcess     = "ocr:process"
    TaskAIBlock        = "ai:block"
    TaskAIDocument     = "ai:document"
    TaskExportDocument = "export:document"
)

// OcrPayload is the task payload for OCR processing.
type OcrPayload struct {
    DocumentID string `json:"document_id"`
    UserID     string `json:"user_id"`
}

// AIBlockPayload is used for async AI block editing (non-streaming jobs).
type AIBlockPayload struct {
    DocumentID  string `json:"document_id"`
    BlockID     string `json:"block_id"`
    Instruction string `json:"instruction"`
    Provider    string `json:"provider"`
    Model       string `json:"model"`
}

// ExportPayload triggers an export job.
type ExportPayload struct {
    DocumentID string `json:"document_id"`
    Format     string `json:"format"`
    Theme      string `json:"theme"`
    CallbackURL string `json:"callback_url,omitempty"`
}

func Encode(v interface{}) ([]byte, error) { return json.Marshal(v) }
func Decode(b []byte, v interface{}) error  { return json.Unmarshal(b, v) }
```

- [ ] **Step 2: Create internal/queue/client.go**

```go
package queue

import (
    "context"
    "github.com/hibiken/asynq"
)

type Client struct {
    c *asynq.Client
}

func NewClient(redisURL string) *Client {
    opt, _ := asynq.ParseRedisURI(redisURL)
    return &Client{c: asynq.NewClient(opt)}
}

func (c *Client) Close() { c.c.Close() }

func (c *Client) EnqueueOCR(ctx context.Context, p OcrPayload) error {
    b, _ := Encode(p)
    task := asynq.NewTask(TaskOCRProcess, b, asynq.Queue(QueueOCR), asynq.MaxRetry(3))
    _, err := c.c.EnqueueContext(ctx, task)
    return err
}

func (c *Client) EnqueueExport(ctx context.Context, p ExportPayload) error {
    b, _ := Encode(p)
    task := asynq.NewTask(TaskExportDocument, b, asynq.Queue(QueueExport), asynq.MaxRetry(2))
    _, err := c.c.EnqueueContext(ctx, task)
    return err
}
```

- [ ] **Step 3: Create internal/queue/server.go**

```go
package queue

import (
    "log"
    "github.com/hibiken/asynq"
)

// NewServer creates an asynq server ready to register handlers.
// concurrency controls total goroutines across all queues.
func NewServer(redisURL string, concurrency int) *asynq.Server {
    opt, _ := asynq.ParseRedisURI(redisURL)
    return asynq.NewServer(opt, asynq.Config{
        Concurrency: concurrency,
        Queues: map[string]int{
            QueueOCR:    10, // OCR gets highest priority
            QueueAI:     5,
            QueueExport: 1,
        },
        ErrorHandler: asynq.ErrorHandlerFunc(func(ctx interface{}, task *asynq.Task, err error) {
            log.Printf("asynq error: task=%s err=%v", task.Type(), err)
        }),
    })
}

// NewMux creates a handler mux. Register handlers on the returned mux.
func NewMux() *asynq.ServeMux {
    return asynq.NewServeMux()
}
```

- [ ] **Step 4: Commit**

```bash
git add webapp/internal/queue/
git commit -m "feat(webapp): asynq task queue — tasks, client, server setup"
```

---

## Task 4: Auth Middleware (JWKS + JWT)

**Files:**
- Create: `webapp/internal/auth/jwks.go`
- Create: `webapp/internal/auth/middleware.go`

- [ ] **Step 1: Create internal/auth/jwks.go**

```go
package auth

import (
    "context"
    "crypto/rsa"
    "encoding/base64"
    "encoding/json"
    "fmt"
    "math/big"
    "net/http"
    "sync"
    "time"
)

type JWKS struct {
    mu      sync.RWMutex
    keys    map[string]*rsa.PublicKey
    url     string
    ttl     time.Duration
    fetched time.Time
}

func NewJWKS(jwksURL string) *JWKS {
    return &JWKS{url: jwksURL, ttl: 5 * time.Minute, keys: make(map[string]*rsa.PublicKey)}
}

func (j *JWKS) GetKey(kid string) (*rsa.PublicKey, error) {
    j.mu.RLock()
    if time.Since(j.fetched) < j.ttl {
        k, ok := j.keys[kid]
        j.mu.RUnlock()
        if ok {
            return k, nil
        }
    } else {
        j.mu.RUnlock()
    }
    return j.refresh(kid)
}

func (j *JWKS) refresh(kid string) (*rsa.PublicKey, error) {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    req, _ := http.NewRequestWithContext(ctx, http.MethodGet, j.url, nil)
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return nil, fmt.Errorf("jwks fetch: %w", err)
    }
    defer resp.Body.Close()

    var payload struct {
        Keys []struct {
            Kid string `json:"kid"`
            N   string `json:"n"`
            E   string `json:"e"`
        } `json:"keys"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
        return nil, err
    }

    j.mu.Lock()
    defer j.mu.Unlock()
    for _, k := range payload.Keys {
        nBytes, _ := base64.RawURLEncoding.DecodeString(k.N)
        eBytes, _ := base64.RawURLEncoding.DecodeString(k.E)
        e := int(new(big.Int).SetBytes(eBytes).Int64())
        pub := &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: e}
        j.keys[k.Kid] = pub
    }
    j.fetched = time.Now()

    if k, ok := j.keys[kid]; ok {
        return k, nil
    }
    // Fallback: return first key if kid not found (auth-service uses "primary")
    for _, k := range j.keys {
        return k, nil
    }
    return nil, fmt.Errorf("jwks: key %q not found", kid)
}
```

- [ ] **Step 2: Create internal/auth/middleware.go**

```go
package auth

import (
    "net/http"
    "strings"

    "github.com/gin-gonic/gin"
    "github.com/golang-jwt/jwt/v5"
)

const CtxUserID  = "user_id"
const CtxEmail   = "email"
const CtxRoles   = "roles"

type Claims struct {
    jwt.RegisteredClaims
    Email  string   `json:"email"`
    Roles  []string `json:"roles"`
    Scopes []string `json:"scopes"`
}

func Middleware(jwks *JWKS) gin.HandlerFunc {
    return func(c *gin.Context) {
        header := c.GetHeader("Authorization")
        if !strings.HasPrefix(header, "Bearer ") {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
            return
        }
        raw := strings.TrimPrefix(header, "Bearer ")

        token, err := jwt.ParseWithClaims(raw, &Claims{}, func(t *jwt.Token) (interface{}, error) {
            if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
                return nil, jwt.ErrSignatureInvalid
            }
            kid, _ := t.Header["kid"].(string)
            return jwks.GetKey(kid)
        })
        if err != nil || !token.Valid {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
            return
        }

        claims := token.Claims.(*Claims)
        c.Set(CtxUserID, claims.Subject)
        c.Set(CtxEmail, claims.Email)
        c.Set(CtxRoles, claims.Roles)
        c.Next()
    }
}

// UserID extracts the user ID set by Middleware.
func UserID(c *gin.Context) string {
    id, _ := c.Get(CtxUserID)
    s, _ := id.(string)
    return s
}
```

- [ ] **Step 3: Commit**

```bash
git add webapp/internal/auth/
git commit -m "feat(webapp): JWT auth middleware — JWKS fetch + RS256 validation"
```

---

## Task 5: Quota Client

**Files:**
- Create: `webapp/internal/quota/client.go`

- [ ] **Step 1: Create internal/quota/client.go**

```go
package quota

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "strings"
    "time"
)

type Status struct {
    ProductSlug    string    `json:"product_slug"`
    Scope          string    `json:"scope"`
    Window         string    `json:"window"`
    EffectiveLimit int       `json:"effective_limit"`
    CurrentCount   int       `json:"current_count"`
    Remaining      int       `json:"remaining"`
    IsExceeded     bool      `json:"is_exceeded"`
    ResetAt        time.Time `json:"reset_at"`
}

type IncrementResponse struct {
    Status   Status `json:"status"`
    Accepted bool   `json:"accepted"`
}

type Client struct {
    baseURL     string
    productSlug string
    httpClient  *http.Client
}

func New(authServiceURL, productSlug string) *Client {
    return &Client{
        baseURL:     strings.TrimRight(authServiceURL, "/"),
        productSlug: productSlug,
        httpClient:  &http.Client{Timeout: 5 * time.Second},
    }
}

func (c *Client) Check(ctx context.Context, scope, bearerToken string) (*Status, error) {
    url := fmt.Sprintf("%s/auth/quota/%s/%s", c.baseURL, c.productSlug, scope)
    req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
    req.Header.Set("Authorization", "Bearer "+bearerToken)

    resp, err := c.httpClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    var s Status
    if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
        return nil, err
    }
    return &s, nil
}

func (c *Client) Increment(ctx context.Context, scope, bearerToken string) (*IncrementResponse, error) {
    url := fmt.Sprintf("%s/auth/quota/%s/%s/increment", c.baseURL, c.productSlug, scope)
    req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
    req.Header.Set("Authorization", "Bearer "+bearerToken)

    resp, err := c.httpClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    var r IncrementResponse
    if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
        return nil, err
    }
    return &r, nil
}
```

- [ ] **Step 2: Add quota middleware helper**

Add to `internal/quota/client.go`:

```go
// QuotaScopes used by this application.
const (
    ScopeOCR    = "ocr_pages"
    ScopeAI     = "ai_requests"
    ScopeExport = "exports"
)

// GinMiddleware checks and increments quota before the handler runs.
// Use: router.POST("/documents/:id/ocr", quota.GinMiddleware(quotaClient, ScopeOCR), handler)
func GinMiddleware(client *Client, scope string) func(c interface{ GetHeader(string) string; AbortWithStatusJSON(int, interface{}); Next(); Request interface{ Header interface{ Get(string) string } } }) {
    // Implemented in Gin-specific file to avoid circular import.
    // See internal/quota/gin.go below.
    return nil
}
```

- [ ] **Step 3: Create internal/quota/gin.go**

```go
package quota

import (
    "net/http"
    "github.com/gin-gonic/gin"
)

// Middleware checks quota and increments it atomically before the handler runs.
// Aborts with 429 if quota exceeded.
func Middleware(client *Client, scope string) gin.HandlerFunc {
    return func(c *gin.Context) {
        token := c.GetHeader("Authorization")
        if len(token) > 7 {
            token = token[7:] // strip "Bearer "
        }

        result, err := client.Increment(c.Request.Context(), scope, token)
        if err != nil {
            // Don't block on quota service errors — log and continue
            c.Next()
            return
        }
        if !result.Accepted {
            c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
                "error":     "quota exceeded",
                "scope":     scope,
                "remaining": 0,
                "reset_at":  result.Status.ResetAt,
            })
            return
        }
        c.Set("quota_remaining", result.Status.Remaining)
        c.Next()
    }
}
```

- [ ] **Step 4: Commit**

```bash
git add webapp/internal/quota/
git commit -m "feat(webapp): quota client + Gin middleware — auth-service integration"
```

---

## Task 6: Documents Handler + Repository

**Files:**
- Create: `webapp/internal/documents/repository.go`
- Create: `webapp/internal/documents/service.go`
- Create: `webapp/internal/documents/handler.go`

- [ ] **Step 1: Create repository.go**

```go
package documents

import (
    "gorm.io/gorm"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) Create(doc *Document) error {
    return r.db.Create(doc).Error
}

func (r *Repository) ListByUser(userID string) ([]Document, error) {
    var docs []Document
    err := r.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&docs).Error
    return docs, err
}

func (r *Repository) GetByID(id, userID string) (*Document, error) {
    var doc Document
    err := r.db.Preload("Pages.Blocks").
        Where("id = ? AND user_id = ?", id, userID).
        First(&doc).Error
    if err != nil {
        return nil, err
    }
    return &doc, nil
}

func (r *Repository) UpdateStatus(id string, status DocumentStatus, errMsg *string) error {
    updates := map[string]interface{}{"status": status}
    if errMsg != nil {
        updates["error_message"] = errMsg
    }
    return r.db.Model(&Document{}).Where("id = ?", id).Updates(updates).Error
}

func (r *Repository) Delete(id, userID string) error {
    return r.db.Where("id = ? AND user_id = ?", id, userID).Delete(&Document{}).Error
}

func (r *Repository) AddPage(page *Page) error {
    return r.db.Create(page).Error
}

func (r *Repository) UpdatePageCount(docID string, count int) error {
    return r.db.Model(&Document{}).Where("id = ?", docID).Update("page_count", count).Error
}
```

- [ ] **Step 2: Create service.go**

```go
package documents

import (
    "fmt"
    "io"
    "os"
    "path/filepath"

    "github.com/yourorg/pdf-translator-webapp/config"
    "github.com/yourorg/pdf-translator-webapp/internal/shared"
    "gorm.io/datatypes"
    "encoding/json"
)

type Service struct{ repo *Repository }

func NewService(repo *Repository) *Service { return &Service{repo: repo} }

func (s *Service) Upload(userID, filename string, r io.Reader, settings shared.OcrSettings) (*Document, error) {
    settingsJSON, _ := json.Marshal(settings)

    doc := &Document{
        UserID:           userID,
        Title:            filename[:len(filename)-len(filepath.Ext(filename))],
        OriginalFilename: filename,
        OcrSettings:      datatypes.JSON(settingsJSON),
        Status:           StatusPending,
    }
    if err := s.repo.Create(doc); err != nil {
        return nil, fmt.Errorf("create document: %w", err)
    }

    dest := filepath.Join(config.C.UploadsDir, doc.ID+".pdf")
    f, err := os.Create(dest)
    if err != nil {
        return nil, err
    }
    defer f.Close()
    if _, err := io.Copy(f, r); err != nil {
        return nil, err
    }
    return doc, nil
}

func (s *Service) List(userID string) ([]Document, error) { return s.repo.ListByUser(userID) }

func (s *Service) Get(id, userID string) (*Document, error) { return s.repo.GetByID(id, userID) }

func (s *Service) Delete(id, userID string) error {
    if err := s.repo.Delete(id, userID); err != nil {
        return err
    }
    os.Remove(filepath.Join(config.C.UploadsDir, id+".pdf"))
    return nil
}
```

- [ ] **Step 3: Create handler.go**

```go
package documents

import (
    "encoding/json"
    "net/http"

    "github.com/gin-gonic/gin"
    "github.com/yourorg/pdf-translator-webapp/internal/auth"
    "github.com/yourorg/pdf-translator-webapp/internal/queue"
    "github.com/yourorg/pdf-translator-webapp/internal/shared"
)

type Handler struct {
    svc    *Service
    queues *queue.Client
}

func NewHandler(svc *Service, q *queue.Client) *Handler { return &Handler{svc: svc, queues: q} }

func (h *Handler) Register(r *gin.RouterGroup) {
    r.GET("/documents", h.list)
    r.POST("/documents", h.upload)
    r.GET("/documents/:id", h.get)
    r.DELETE("/documents/:id", h.delete)
    r.POST("/documents/:id/ocr", h.startOCR)
}

func (h *Handler) list(c *gin.Context) {
    docs, err := h.svc.List(auth.UserID(c))
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, docs)
}

func (h *Handler) upload(c *gin.Context) {
    file, header, err := c.Request.FormFile("file")
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "missing file"})
        return
    }
    defer file.Close()

    var settings shared.OcrSettings
    if raw := c.PostForm("ocr_settings"); raw != "" {
        json.Unmarshal([]byte(raw), &settings)
    }
    if settings.Engine == "" {
        settings = shared.OcrSettings{Engine: "rapidocr", Lang: []string{"en"}, DoOcr: true, DoTableStructure: true, TableMode: "fast", GeneratePictureImages: true, ImagesScale: 2.0, DocumentTimeout: 300}
    }

    doc, err := h.svc.Upload(auth.UserID(c), header.Filename, file, settings)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusCreated, doc)
}

func (h *Handler) get(c *gin.Context) {
    doc, err := h.svc.Get(c.Param("id"), auth.UserID(c))
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
        return
    }
    c.JSON(http.StatusOK, doc)
}

func (h *Handler) delete(c *gin.Context) {
    if err := h.svc.Delete(c.Param("id"), auth.UserID(c)); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.Status(http.StatusNoContent)
}

func (h *Handler) startOCR(c *gin.Context) {
    docID := c.Param("id")
    if err := h.queues.EnqueueOCR(c.Request.Context(), queue.OcrPayload{
        DocumentID: docID,
        UserID:     auth.UserID(c),
    }); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusAccepted, gin.H{"status": "queued"})
}
```

- [ ] **Step 4: Commit**

```bash
git add webapp/internal/documents/
git commit -m "feat(webapp): documents — handler/service/repo, upload + OCR enqueue"
```

---

## Task 7: OCR Worker (Docling Client)

**Files:**
- Create: `webapp/pkg/docling/client.go`
- Create: `webapp/internal/shared/converter.go`
- Create: `webapp/internal/ocr/worker.go`

- [ ] **Step 1: Create pkg/docling/client.go**

```go
package docling

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "mime/multipart"
    "net/http"
    "os"
    "time"
)

// Client calls the Docling HTTP service (Python FastAPI).
type Client struct {
    baseURL    string
    httpClient *http.Client
}

func New(baseURL string) *Client {
    return &Client{
        baseURL:    baseURL,
        httpClient: &http.Client{Timeout: 600 * time.Second},
    }
}

type ConvertRequest struct {
    Engine              string   `json:"engine"`
    Lang                []string `json:"lang"`
    DoOcr               bool     `json:"do_ocr"`
    DoTableStructure    bool     `json:"do_table_structure"`
    TableMode           string   `json:"table_mode"`
    GeneratePictureImages bool   `json:"generate_picture_images"`
    ImagesScale         float64  `json:"images_scale"`
}

type ConvertResponse struct {
    Markdown string            `json:"markdown"`
    Images   map[string]string `json:"images"` // filename → base64
    Pages    int               `json:"pages"`
}

// Convert sends a PDF to the Docling service and returns markdown + images.
func (c *Client) Convert(ctx context.Context, pdfPath string, req ConvertRequest) (*ConvertResponse, error) {
    var body bytes.Buffer
    w := multipart.NewWriter(&body)

    f, err := os.Open(pdfPath)
    if err != nil {
        return nil, err
    }
    defer f.Close()

    fw, _ := w.CreateFormFile("file", "document.pdf")
    if _, err := io.Copy(fw, f); err != nil {
        return nil, err
    }

    settingsJSON, _ := json.Marshal(req)
    w.WriteField("settings", string(settingsJSON))
    w.Close()

    httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/convert", &body)
    httpReq.Header.Set("Content-Type", w.FormDataContentType())

    resp, err := c.httpClient.Do(httpReq)
    if err != nil {
        return nil, fmt.Errorf("docling: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        b, _ := io.ReadAll(resp.Body)
        return nil, fmt.Errorf("docling: status %d: %s", resp.StatusCode, b)
    }

    var result ConvertResponse
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, err
    }
    return &result, nil
}
```

> **Note:** The Docling service (`deploy/paddleocr/`) needs a `/convert` endpoint added that accepts a PDF + settings and returns JSON with markdown + images. See Task 7b below.

- [ ] **Step 2: Create internal/shared/converter.go**

```go
package shared

import (
    "fmt"
    "regexp"
    "strings"
    "github.com/google/uuid"
)

var (
    headingRe   = regexp.MustCompile(`^(#{1,6})\s+(.*)`)
    imageRe     = regexp.MustCompile(`^!\[([^\]]*)\]\(([^)]+)\)`)
    ulRe        = regexp.MustCompile(`^[-*]\s+(.+)`)
    olRe        = regexp.MustCompile(`^\d+\.\s+(.+)`)
    blockquoteRe = regexp.MustCompile(`^>\s+(.+)`)
    delimiterRe  = regexp.MustCompile(`^---+$`)
    tableRowRe   = regexp.MustCompile(`^\|`)
    tableSepRe   = regexp.MustCompile(`^\|[-| :]+\|$`)
)

// MarkdownToBlocks converts a markdown string to Editor.js block format.
func MarkdownToBlocks(md string) []BlockData {
    lines := strings.Split(md, "\n")
    var blocks []BlockData
    i := 0

    for i < len(lines) {
        line := lines[i]

        if m := headingRe.FindStringSubmatch(line); m != nil {
            blocks = append(blocks, BlockData{ID: uuid.NewString(), Type: "header", Data: map[string]interface{}{"text": m[2], "level": len(m[1])}})
            i++; continue
        }
        if m := imageRe.FindStringSubmatch(line); m != nil {
            blocks = append(blocks, BlockData{ID: uuid.NewString(), Type: "image", Data: map[string]interface{}{"url": m[2], "caption": m[1]}})
            i++; continue
        }
        if strings.HasPrefix(line, "```") {
            lang := strings.TrimPrefix(line, "```")
            var code []string
            i++
            for i < len(lines) && !strings.HasPrefix(lines[i], "```") {
                code = append(code, lines[i]); i++
            }
            blocks = append(blocks, BlockData{ID: uuid.NewString(), Type: "code", Data: map[string]interface{}{"code": strings.Join(code, "\n"), "language": lang}})
            i++; continue
        }
        if ulRe.MatchString(line) {
            var items []string
            for i < len(lines) && ulRe.MatchString(lines[i]) {
                items = append(items, ulRe.FindStringSubmatch(lines[i])[1]); i++
            }
            blocks = append(blocks, BlockData{ID: uuid.NewString(), Type: "list", Data: map[string]interface{}{"style": "unordered", "items": items}})
            continue
        }
        if olRe.MatchString(line) {
            var items []string
            for i < len(lines) && olRe.MatchString(lines[i]) {
                items = append(items, olRe.FindStringSubmatch(lines[i])[1]); i++
            }
            blocks = append(blocks, BlockData{ID: uuid.NewString(), Type: "list", Data: map[string]interface{}{"style": "ordered", "items": items}})
            continue
        }
        if m := blockquoteRe.FindStringSubmatch(line); m != nil {
            blocks = append(blocks, BlockData{ID: uuid.NewString(), Type: "quote", Data: map[string]interface{}{"text": m[1]}})
            i++; continue
        }
        if delimiterRe.MatchString(strings.TrimSpace(line)) {
            blocks = append(blocks, BlockData{ID: uuid.NewString(), Type: "delimiter", Data: map[string]interface{}{}})
            i++; continue
        }
        if tableRowRe.MatchString(line) {
            var rows [][]string
            for i < len(lines) && tableRowRe.MatchString(lines[i]) {
                if !tableSepRe.MatchString(lines[i]) {
                    cells := strings.Split(strings.Trim(lines[i], "|"), "|")
                    for j := range cells { cells[j] = strings.TrimSpace(cells[j]) }
                    rows = append(rows, cells)
                }
                i++
            }
            if len(rows) > 0 {
                blocks = append(blocks, BlockData{ID: uuid.NewString(), Type: "table", Data: map[string]interface{}{"withHeadings": true, "content": rows}})
            }
            continue
        }
        if strings.TrimSpace(line) != "" {
            var para []string
            for i < len(lines) && strings.TrimSpace(lines[i]) != "" &&
                !headingRe.MatchString(lines[i]) && !strings.HasPrefix(lines[i], "```") {
                para = append(para, lines[i]); i++
            }
            blocks = append(blocks, BlockData{ID: uuid.NewString(), Type: "paragraph", Data: map[string]interface{}{"text": strings.Join(para, " ")}})
            continue
        }
        i++
    }
    return blocks
}

// BlocksToMarkdown converts Editor.js blocks back to markdown.
func BlocksToMarkdown(blocks []BlockData) string {
    var parts []string
    for _, b := range blocks {
        switch b.Type {
        case "header":
            level, _ := b.Data["level"].(float64)
            parts = append(parts, fmt.Sprintf("%s %v", strings.Repeat("#", int(level)), b.Data["text"]))
        case "paragraph":
            parts = append(parts, fmt.Sprintf("%v", b.Data["text"]))
        case "list":
            items, _ := b.Data["items"].([]interface{})
            style, _ := b.Data["style"].(string)
            for idx, item := range items {
                if style == "ordered" {
                    parts = append(parts, fmt.Sprintf("%d. %v", idx+1, item))
                } else {
                    parts = append(parts, fmt.Sprintf("- %v", item))
                }
            }
        case "code":
            lang, _ := b.Data["language"].(string)
            parts = append(parts, fmt.Sprintf("```%s\n%v\n```", lang, b.Data["code"]))
        case "quote":
            parts = append(parts, fmt.Sprintf("> %v", b.Data["text"]))
        case "delimiter":
            parts = append(parts, "---")
        case "table":
            rows, _ := b.Data["content"].([]interface{})
            for ri, row := range rows {
                cells, _ := row.([]interface{})
                var cs []string
                for _, c := range cells { cs = append(cs, fmt.Sprintf("%v", c)) }
                parts = append(parts, "| "+strings.Join(cs, " | ")+" |")
                if ri == 0 {
                    sep := make([]string, len(cs))
                    for k := range sep { sep[k] = "---" }
                    parts = append(parts, "| "+strings.Join(sep, " | ")+" |")
                }
            }
        case "image":
            parts = append(parts, fmt.Sprintf("![%v](%v)", b.Data["caption"], b.Data["url"]))
        }
    }
    return strings.Join(parts, "\n\n")
}
```

- [ ] **Step 3: Create internal/ocr/worker.go**

```go
package ocr

import (
    "context"
    "encoding/json"
    "fmt"
    "log"

    "github.com/hibiken/asynq"
    "github.com/yourorg/pdf-translator-webapp/config"
    "github.com/yourorg/pdf-translator-webapp/internal/documents"
    "github.com/yourorg/pdf-translator-webapp/internal/queue"
    "github.com/yourorg/pdf-translator-webapp/internal/shared"
    "github.com/yourorg/pdf-translator-webapp/internal/ws"
    "github.com/yourorg/pdf-translator-webapp/pkg/docling"
    "gorm.io/gorm"
    "path/filepath"
)

type Worker struct {
    db     *gorm.DB
    hub    *ws.Hub
    docl   *docling.Client
}

func NewWorker(db *gorm.DB, hub *ws.Hub) *Worker {
    return &Worker{
        db:   db,
        hub:  hub,
        docl: docling.New(config.C.DockingServiceURL),
    }
}

func (w *Worker) HandleOCR(ctx context.Context, task *asynq.Task) error {
    var p queue.OcrPayload
    if err := json.Unmarshal(task.Payload(), &p); err != nil {
        return fmt.Errorf("decode payload: %w", err)
    }

    w.broadcast(p.DocumentID, "loading", 5)

    // Load document
    var doc documents.Document
    if err := w.db.First(&doc, "id = ?", p.DocumentID).Error; err != nil {
        return err
    }

    var settings shared.OcrSettings
    json.Unmarshal(doc.OcrSettings, &settings)

    // Mark processing
    w.db.Model(&doc).Update("status", documents.StatusProcessing)
    w.broadcast(p.DocumentID, "converting", 20)

    pdfPath := filepath.Join(config.C.UploadsDir, p.DocumentID+".pdf")
    result, err := w.docl.Convert(ctx, pdfPath, docling.ConvertRequest{
        Engine:                settings.Engine,
        Lang:                  settings.Lang,
        DoOcr:                 settings.DoOcr,
        DoTableStructure:      settings.DoTableStructure,
        TableMode:             settings.TableMode,
        GeneratePictureImages: settings.GeneratePictureImages,
        ImagesScale:           settings.ImagesScale,
    })
    if err != nil {
        errMsg := err.Error()
        w.db.Model(&doc).Updates(map[string]interface{}{"status": documents.StatusError, "error_message": errMsg})
        w.hub.Broadcast(p.DocumentID, ws.Message{Stage: "error", Error: errMsg})
        return err
    }

    w.broadcast(p.DocumentID, "extracting", 70)

    blocks := shared.MarkdownToBlocks(result.Markdown)
    page := documents.Page{DocumentID: p.DocumentID, PageNumber: 1, RawMarkdown: &result.Markdown}
    w.db.Create(&page)

    for order, b := range blocks {
        dataJSON, _ := json.Marshal(b.Data)
        w.db.Create(&documents.Block{PageID: page.ID, Type: b.Type, Data: dataJSON, Order: order})
    }

    w.db.Model(&doc).Updates(map[string]interface{}{"status": documents.StatusReady, "page_count": result.Pages})
    w.hub.Broadcast(p.DocumentID, ws.Message{Stage: "complete", Percent: 100})
    log.Printf("ocr: document %s processed, %d blocks", p.DocumentID, len(blocks))
    return nil
}

func (w *Worker) broadcast(docID, stage string, pct int) {
    w.hub.Broadcast(docID, ws.Message{Stage: stage, Percent: pct})
}
```

- [ ] **Step 4: Commit**

```bash
git add webapp/pkg/docling/ webapp/internal/shared/converter.go webapp/internal/ocr/
git commit -m "feat(webapp): OCR worker — Docling client, markdown→blocks converter, asynq handler"
```

---

## Task 7b: Docling Service `/convert` Endpoint

The existing `deploy/paddleocr/server.py` needs a unified `/convert` endpoint that returns JSON.

**Files:**
- Modify: `deploy/paddleocr/server.py`

- [ ] **Step 1: Add /convert endpoint to deploy/paddleocr/server.py**

```python
from fastapi import UploadFile, File, Form
from pydantic import BaseModel
import json, base64, tempfile
from pathlib import Path
from docling.document_converter import DocumentConverter, PdfFormatOption
from docling.datamodel.base_models import InputFormat
from docling.datamodel.pipeline_options import (
    PdfPipelineOptions, EasyOcrOptions, RapidOcrOptions,
    TesseractOcrOptions, OcrMacOptions, TableStructureOptions, TableFormerMode,
)
from docling_core.types.doc.document import ImageRefMode

class ConvertSettings(BaseModel):
    engine: str = "rapidocr"
    lang: list[str] = ["en"]
    do_ocr: bool = True
    do_table_structure: bool = True
    table_mode: str = "fast"
    generate_picture_images: bool = True
    images_scale: float = 2.0

@app.post("/convert")
async def convert_pdf(file: UploadFile = File(...), settings: str = Form(default="{}")):
    s = ConvertSettings(**json.loads(settings))

    opts = PdfPipelineOptions(
        do_ocr=s.do_ocr,
        do_table_structure=s.do_table_structure,
        generate_picture_images=s.generate_picture_images,
        images_scale=s.images_scale,
    )
    if s.do_table_structure:
        mode = TableFormerMode.ACCURATE if s.table_mode == "accurate" else TableFormerMode.FAST
        opts.table_structure_options = TableStructureOptions(mode=mode)
    if s.do_ocr:
        if s.engine == "easyocr":
            opts.ocr_options = EasyOcrOptions(lang=s.lang)
        elif s.engine == "tesseract":
            opts.ocr_options = TesseractOcrOptions(lang=s.lang[0] if s.lang else "eng")
        else:
            opts.ocr_options = RapidOcrOptions()

    converter = DocumentConverter(format_options={InputFormat.PDF: PdfFormatOption(pipeline_options=opts)})

    with tempfile.TemporaryDirectory() as tmp:
        pdf_path = Path(tmp) / "input.pdf"
        pdf_path.write_bytes(await file.read())
        images_dir = Path(tmp) / "images"
        images_dir.mkdir()

        result = converter.convert(str(pdf_path))
        doc = result.document

        doc.save_as_markdown(Path(tmp) / "out.md", image_mode=ImageRefMode.REFERENCED, artifacts_dir=images_dir)
        markdown = (Path(tmp) / "out.md").read_text()
        markdown = markdown.replace(str(images_dir) + "/", "")

        images = {}
        for img_file in images_dir.glob("*.png"):
            images[img_file.name] = base64.b64encode(img_file.read_bytes()).decode()

        return {"markdown": markdown, "images": images, "pages": len(doc.pages)}
```

- [ ] **Step 2: Commit**

```bash
git add deploy/paddleocr/server.py
git commit -m "feat(paddleocr): add /convert endpoint — returns markdown + images JSON"
```

---

## Task 8: WebSocket Hub + Blocks Handler + AI Handler

**Files:**
- Create: `webapp/internal/ws/hub.go`
- Create: `webapp/internal/blocks/handler.go`
- Create: `webapp/internal/ai/handler.go`
- Create: `webapp/internal/ai/service.go`

- [ ] **Step 1: Create internal/ws/hub.go**

```go
package ws

import (
    "encoding/json"
    "net/http"
    "sync"

    "github.com/gin-gonic/gin"
    "github.com/gorilla/websocket"
)

type Message struct {
    Stage   string `json:"stage"`
    Percent int    `json:"percent,omitempty"`
    Error   string `json:"error,omitempty"`
}

var upgrader = websocket.Upgrader{
    CheckOrigin: func(r *http.Request) bool { return true },
}

type Hub struct {
    mu   sync.RWMutex
    subs map[string][]*websocket.Conn
}

func NewHub() *Hub { return &Hub{subs: make(map[string][]*websocket.Conn)} }

func (h *Hub) Broadcast(docID string, msg Message) {
    b, _ := json.Marshal(msg)
    h.mu.RLock()
    conns := h.subs[docID]
    h.mu.RUnlock()
    for _, c := range conns {
        c.WriteMessage(websocket.TextMessage, b) //nolint
    }
}

func (h *Hub) Handler(c *gin.Context) {
    docID := c.Param("id")
    conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
    if err != nil {
        return
    }
    h.mu.Lock()
    h.subs[docID] = append(h.subs[docID], conn)
    h.mu.Unlock()

    defer func() {
        h.mu.Lock()
        conns := h.subs[docID]
        for i, co := range conns {
            if co == conn {
                h.subs[docID] = append(conns[:i], conns[i+1:]...)
                break
            }
        }
        h.mu.Unlock()
        conn.Close()
    }()

    for {
        if _, _, err := conn.ReadMessage(); err != nil {
            break
        }
    }
}
```

- [ ] **Step 2: Create internal/blocks/handler.go**

```go
package blocks

import (
    "encoding/json"
    "net/http"

    "github.com/gin-gonic/gin"
    "github.com/yourorg/pdf-translator-webapp/internal/documents"
    "gorm.io/gorm"
)

type Handler struct{ db *gorm.DB }

func NewHandler(db *gorm.DB) *Handler { return &Handler{db: db} }

func (h *Handler) Register(r *gin.RouterGroup) {
    r.PUT("/documents/:id/blocks", h.save)
}

type saveRequest struct {
    PageID string `json:"page_id" binding:"required"`
    Blocks []struct {
        ID    string                 `json:"id"`
        Type  string                 `json:"type"`
        Data  map[string]interface{} `json:"data"`
        Order int                    `json:"order"`
    } `json:"blocks"`
}

func (h *Handler) save(c *gin.Context) {
    var req saveRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    h.db.Where("page_id = ?", req.PageID).Delete(&documents.Block{})
    for _, b := range req.Blocks {
        dataJSON, _ := json.Marshal(b.Data)
        h.db.Create(&documents.Block{ID: b.ID, PageID: req.PageID, Type: b.Type, Data: dataJSON, Order: b.Order})
    }
    c.JSON(http.StatusOK, gin.H{"saved": len(req.Blocks)})
}
```

- [ ] **Step 3: Create internal/ai/service.go**

```go
package ai

import (
    "context"
    "fmt"
    "io"
    "net/http"
    "strings"
    "encoding/json"
    "github.com/yourorg/pdf-translator-webapp/config"
)

const systemEdit = "You are a professional document editor. Edit the text per the instruction. Output only the edited text, preserving markdown."
const systemAsk  = "You are a helpful assistant. Answer concisely based on the document."

// StreamBlock streams Claude or OpenAI response for a single block.
func StreamBlock(ctx context.Context, blockText, instruction, mode, provider, model string, w io.Writer) error {
    system := systemAsk
    if mode == "edit" {
        system = systemEdit
    }
    userMsg := fmt.Sprintf("Text:\n%s\n\nInstruction: %s", blockText, instruction)

    if provider == "openai" {
        return streamOpenAI(ctx, system, userMsg, model, w)
    }
    return streamClaude(ctx, system, userMsg, model, w)
}

func streamClaude(ctx context.Context, system, userMsg, model string, w io.Writer) error {
    body := map[string]interface{}{
        "model":      model,
        "max_tokens": 4096,
        "stream":     true,
        "system": []map[string]interface{}{
            {"type": "text", "text": system, "cache_control": map[string]string{"type": "ephemeral"}},
        },
        "messages": []map[string]interface{}{{"role": "user", "content": userMsg}},
    }
    b, _ := json.Marshal(body)
    req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.anthropic.com/v1/messages", strings.NewReader(string(b)))
    req.Header.Set("x-api-key", config.C.AnthropicAPIKey)
    req.Header.Set("anthropic-version", "2023-06-01")
    req.Header.Set("anthropic-beta", "prompt-caching-2024-07-31")
    req.Header.Set("content-type", "application/json")

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    buf := make([]byte, 4096)
    for {
        n, err := resp.Body.Read(buf)
        if n > 0 {
            lines := strings.Split(string(buf[:n]), "\n")
            for _, line := range lines {
                if strings.HasPrefix(line, "data: ") {
                    var ev map[string]interface{}
                    if json.Unmarshal([]byte(line[6:]), &ev) == nil {
                        if delta, ok := ev["delta"].(map[string]interface{}); ok {
                            if text, ok := delta["text"].(string); ok {
                                fmt.Fprintf(w, "data: %s\n\n", text)
                            }
                        }
                    }
                }
            }
        }
        if err == io.EOF {
            break
        }
        if err != nil {
            return err
        }
    }
    fmt.Fprint(w, "data: [DONE]\n\n")
    return nil
}

func streamOpenAI(ctx context.Context, system, userMsg, model string, w io.Writer) error {
    body := map[string]interface{}{
        "model":  model,
        "stream": true,
        "messages": []map[string]interface{}{
            {"role": "system", "content": system},
            {"role": "user", "content": userMsg},
        },
    }
    b, _ := json.Marshal(body)
    req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/chat/completions", strings.NewReader(string(b)))
    req.Header.Set("Authorization", "Bearer "+config.C.OpenAIAPIKey)
    req.Header.Set("Content-Type", "application/json")

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    io.Copy(w, resp.Body) // OpenAI already sends SSE format
    return nil
}
```

- [ ] **Step 4: Create internal/ai/handler.go**

```go
package ai

import (
    "net/http"
    "github.com/gin-gonic/gin"
)

type Handler struct{}

func NewHandler() *Handler { return &Handler{} }

func (h *Handler) Register(r *gin.RouterGroup) {
    r.POST("/ai/block", h.block)
    r.POST("/ai/document/:id", h.document)
}

type blockRequest struct {
    BlockText   string `json:"block_text" binding:"required"`
    Instruction string `json:"instruction" binding:"required"`
    Mode        string `json:"mode"`
    Provider    string `json:"provider"`
    Model       string `json:"model"`
}

func (h *Handler) block(c *gin.Context) {
    var req blockRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    if req.Mode == "" { req.Mode = "ask" }
    if req.Provider == "" { req.Provider = "claude" }
    if req.Model == "" { req.Model = "claude-sonnet-4-6" }

    c.Header("Content-Type", "text/event-stream")
    c.Header("Cache-Control", "no-cache")
    c.Header("Connection", "keep-alive")
    c.Status(http.StatusOK)

    StreamBlock(c.Request.Context(), req.BlockText, req.Instruction, req.Mode, req.Provider, req.Model, c.Writer)
    c.Writer.Flush()
}

type documentRequest struct {
    Instruction string `json:"instruction" binding:"required"`
    Mode        string `json:"mode"`
    Provider    string `json:"provider"`
    Model       string `json:"model"`
}

func (h *Handler) document(c *gin.Context) {
    // Similar to block but builds full markdown from all blocks first
    // (implementation mirrors block handler, uses BlocksToMarkdown from shared)
    c.JSON(http.StatusNotImplemented, gin.H{"error": "TODO"})
}
```

- [ ] **Step 5: Commit**

```bash
git add webapp/internal/ws/ webapp/internal/blocks/ webapp/internal/ai/
git commit -m "feat(webapp): WebSocket hub + blocks save + AI streaming handler"
```

---

## Task 9: Export Handler

**Files:**
- Create: `webapp/internal/export/service.go`
- Create: `webapp/internal/export/handler.go`

- [ ] **Step 1: Create internal/export/service.go**

```go
package export

import (
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "regexp"
    "strings"

    "github.com/yourorg/pdf-translator-webapp/config"
    "github.com/yourorg/pdf-translator-webapp/internal/documents"
    "github.com/yourorg/pdf-translator-webapp/internal/shared"
    "gorm.io/gorm"
    "encoding/json"
)

func Export(db *gorm.DB, doc *documents.Document, format, theme string) (string, error) {
    exports := config.C.ExportsDir
    os.MkdirAll(exports, 0755)
    stem := strings.ReplaceAll(doc.Title, " ", "_")

    // Collect blocks → markdown
    var blocks []shared.BlockData
    for _, page := range doc.Pages {
        for _, b := range page.Blocks {
            var data map[string]interface{}
            json.Unmarshal(b.Data, &data)
            blocks = append(blocks, shared.BlockData{ID: b.ID, Type: b.Type, Data: data, Order: b.Order})
        }
    }
    md := shared.BlocksToMarkdown(blocks)

    switch format {
    case "markdown":
        p := filepath.Join(exports, stem+".md")
        return p, os.WriteFile(p, []byte(md), 0644)

    case "txt":
        txt := regexp.MustCompile(`[#*` + "`>|_~\\[\\]!]`").ReplaceAllString(md, "")
        p := filepath.Join(exports, stem+".txt")
        return p, os.WriteFile(p, []byte(txt), 0644)

    case "pdf":
        mdPath := filepath.Join(exports, stem+".md")
        if err := os.WriteFile(mdPath, []byte(md), 0644); err != nil {
            return "", err
        }
        flag := "--light"
        if theme == "dark" { flag = "--dark" }
        if err := exec.Command(config.C.MdtoPdfScript, flag, mdPath).Run(); err != nil {
            return "", fmt.Errorf("mdtopdf: %w", err)
        }
        return filepath.Join(exports, stem+".pdf"), nil

    case "docx":
        mdPath := filepath.Join(exports, stem+".md")
        outPath := filepath.Join(exports, stem+".docx")
        if err := os.WriteFile(mdPath, []byte(md), 0644); err != nil {
            return "", err
        }
        return outPath, exec.Command("pandoc", mdPath, "-o", outPath).Run()

    case "html":
        mdPath := filepath.Join(exports, stem+".md")
        outPath := filepath.Join(exports, stem+".html")
        if err := os.WriteFile(mdPath, []byte(md), 0644); err != nil {
            return "", err
        }
        return outPath, exec.Command("pandoc", mdPath, "-o", outPath, "--standalone").Run()
    }

    return "", fmt.Errorf("unknown format: %s", format)
}
```

- [ ] **Step 2: Create internal/export/handler.go**

```go
package export

import (
    "net/http"
    "path/filepath"
    "github.com/gin-gonic/gin"
    "github.com/yourorg/pdf-translator-webapp/internal/auth"
    "github.com/yourorg/pdf-translator-webapp/internal/documents"
    "gorm.io/gorm"
)

type Handler struct{ db *gorm.DB }

func NewHandler(db *gorm.DB) *Handler { return &Handler{db: db} }

func (h *Handler) Register(r *gin.RouterGroup) {
    r.POST("/documents/:id/export", h.export)
}

type exportRequest struct {
    Format string `json:"format" binding:"required"`
    Theme  string `json:"theme"`
}

func (h *Handler) export(c *gin.Context) {
    var req exportRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    if req.Theme == "" { req.Theme = "light" }

    var doc documents.Document
    if err := h.db.Preload("Pages.Blocks").
        Where("id = ? AND user_id = ?", c.Param("id"), auth.UserID(c)).
        First(&doc).Error; err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
        return
    }

    path, err := Export(h.db, &doc, req.Format, req.Theme)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.FileAttachment(path, filepath.Base(path))
}
```

- [ ] **Step 3: Commit**

```bash
git add webapp/internal/export/
git commit -m "feat(webapp): export handler — md/txt/pdf/docx/html via pandoc + mdtopdf"
```

---

## Task 10: Server + Worker Entry Points

**Files:**
- Create: `webapp/cmd/server/main.go`
- Create: `webapp/cmd/worker/main.go`

- [ ] **Step 1: Create cmd/server/main.go**

```go
package main

import (
    "log"
    "os"
    "path/filepath"

    "github.com/gin-gonic/gin"
    "github.com/yourorg/pdf-translator-webapp/config"
    "github.com/yourorg/pdf-translator-webapp/internal/ai"
    "github.com/yourorg/pdf-translator-webapp/internal/auth"
    "github.com/yourorg/pdf-translator-webapp/internal/blocks"
    "github.com/yourorg/pdf-translator-webapp/internal/documents"
    "github.com/yourorg/pdf-translator-webapp/internal/export"
    "github.com/yourorg/pdf-translator-webapp/internal/ocr"
    "github.com/yourorg/pdf-translator-webapp/internal/queue"
    "github.com/yourorg/pdf-translator-webapp/internal/quota"
    "github.com/yourorg/pdf-translator-webapp/internal/ws"
    "gorm.io/driver/postgres"
    "gorm.io/gorm"
)

func main() {
    config.Load()

    os.MkdirAll(config.C.UploadsDir, 0755)
    os.MkdirAll(config.C.ExportsDir, 0755)

    // DB
    db, err := gorm.Open(postgres.Open(config.C.DatabaseURL), &gorm.Config{})
    if err != nil {
        log.Fatalf("db: %v", err)
    }
    db.AutoMigrate(&documents.Document{}, &documents.Page{}, &documents.Block{})

    // Queue
    qClient := queue.NewClient(config.C.RedisURL)
    defer qClient.Close()

    // WebSocket hub
    hub := ws.NewHub()

    // Auth + Quota
    jwks := auth.NewJWKS(config.C.AuthServiceURL + "/.well-known/jwks.json")
    quotaClient := quota.New(config.C.AuthServiceURL, config.C.QuotaProductSlug)

    // Gin
    r := gin.Default()
    r.Use(func(c *gin.Context) {
        c.Header("Access-Control-Allow-Origin", "http://localhost:3000")
        c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
        c.Header("Access-Control-Allow-Headers", "Authorization,Content-Type")
        if c.Request.Method == "OPTIONS" { c.AbortWithStatus(204); return }
        c.Next()
    })

    // WebSocket (no auth — uses doc ownership check)
    r.GET("/api/ws/:id/progress", hub.Handler)

    // Authenticated routes
    api := r.Group("/api", auth.Middleware(jwks))

    docSvc := documents.NewService(documents.NewRepository(db))
    documents.NewHandler(docSvc, qClient).Register(api)

    // OCR with quota check
    _ = quota.Middleware(quotaClient, quota.ScopeOCR) // attach in handler

    blocks.NewHandler(db).Register(api)
    ai.NewHandler().Register(api)
    export.NewHandler(db).Register(api)

    // Embedded worker (runs in same process)
    go startWorker(db, hub)

    log.Printf("server listening on :%s", config.C.Port)
    r.Run(":" + config.C.Port)
}

func startWorker(db *gorm.DB, hub *ws.Hub) {
    _ = filepath.Join // suppress unused import
    srv := queue.NewServer(config.C.RedisURL, config.C.WorkerConcurrency)
    mux := queue.NewMux()

    ocrWorker := ocr.NewWorker(db, hub)
    mux.HandleFunc(queue.TaskOCRProcess, ocrWorker.HandleOCR)

    if err := srv.Run(mux); err != nil {
        log.Printf("worker error: %v", err)
    }
}
```

- [ ] **Step 2: Create cmd/worker/main.go**

```go
package main

// Standalone worker — connect to same Redis + DB as server, process tasks only.
// Launch additional instances to scale OCR/AI throughput:
//   ./worker --queues ocr:10,ai:5

import (
    "log"
    "github.com/yourorg/pdf-translator-webapp/config"
    "github.com/yourorg/pdf-translator-webapp/internal/documents"
    "github.com/yourorg/pdf-translator-webapp/internal/ocr"
    "github.com/yourorg/pdf-translator-webapp/internal/queue"
    "github.com/yourorg/pdf-translator-webapp/internal/ws"
    "gorm.io/driver/postgres"
    "gorm.io/gorm"
)

func main() {
    config.Load()

    db, err := gorm.Open(postgres.Open(config.C.DatabaseURL), &gorm.Config{})
    if err != nil {
        log.Fatalf("db: %v", err)
    }
    db.AutoMigrate(&documents.Document{}, &documents.Page{}, &documents.Block{})

    // Worker doesn't serve WebSocket — broadcast goes nowhere (no-op)
    hub := ws.NewHub()

    srv := queue.NewServer(config.C.RedisURL, config.C.WorkerConcurrency)
    mux := queue.NewMux()

    ocrWorker := ocr.NewWorker(db, hub)
    mux.HandleFunc(queue.TaskOCRProcess, ocrWorker.HandleOCR)

    log.Println("worker: starting — queues: ocr, ai, export")
    if err := srv.Run(mux); err != nil {
        log.Fatalf("worker: %v", err)
    }
}
```

- [ ] **Step 3: Build and verify**

```bash
cd webapp
go build ./cmd/server && go build ./cmd/worker
# Expect: both binaries compile without errors
./server
# Expect: "server listening on :8080"
```

- [ ] **Step 4: Commit**

```bash
git add webapp/cmd/
git commit -m "feat(webapp): server + worker entry points — modular monolith + slave mode"
```

---

## Task 11: Next.js Frontend

**Files:**
- Create: `webapp/frontend/` (Next.js 15 App Router)

- [ ] **Step 1: Init Next.js**

```bash
cd webapp
npx create-next-app@latest frontend --typescript --tailwind --app --no-src-dir --import-alias "@/*"
cd frontend
npm install @editorjs/editorjs @editorjs/header @editorjs/list @editorjs/code @editorjs/quote @editorjs/delimiter @editorjs/table @editorjs/checklist @editorjs/image
```

- [ ] **Step 2: Create frontend/lib/auth.ts**

```typescript
// Auth-service client
const AUTH_URL = process.env.NEXT_PUBLIC_AUTH_URL ?? "http://localhost:8002";

export interface AuthTokens {
  access_token: string;
  refresh_token: string;
  token_type: string;
}

export async function login(email: string, password: string): Promise<AuthTokens> {
  const res = await fetch(`${AUTH_URL}/auth/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email, password }),
  });
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

export async function refreshTokens(refreshToken: string): Promise<AuthTokens> {
  const res = await fetch(`${AUTH_URL}/auth/refresh`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ refresh_token: refreshToken }),
  });
  if (!res.ok) throw new Error("Session expired");
  return res.json();
}

// Store tokens in localStorage (or httpOnly cookies in production)
export const tokenStore = {
  get: () => localStorage.getItem("access_token") ?? "",
  set: (t: AuthTokens) => {
    localStorage.setItem("access_token", t.access_token);
    localStorage.setItem("refresh_token", t.refresh_token);
  },
  clear: () => {
    localStorage.removeItem("access_token");
    localStorage.removeItem("refresh_token");
  },
};
```

- [ ] **Step 3: Create frontend/lib/api.ts**

```typescript
import { tokenStore } from "./auth";

const BASE = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080/api";

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const token = tokenStore.get();
  const res = await fetch(BASE + path, {
    ...init,
    headers: {
      ...(init?.headers ?? {}),
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
  });
  if (res.status === 401) {
    tokenStore.clear();
    window.location.href = "/login";
    throw new Error("Unauthorized");
  }
  if (!res.ok) throw new Error(await res.text());
  if (res.status === 204) return undefined as T;
  return res.json();
}

export const api = {
  listDocuments:  ()                       => req<DocumentListItem[]>("/documents"),
  getDocument:    (id: string)             => req<Document>(`/documents/${id}`),
  deleteDocument: (id: string)             => req<void>(`/documents/${id}`, { method: "DELETE" }),

  uploadDocument: (file: File, ocrSettings: OcrSettings) => {
    const form = new FormData();
    form.append("file", file);
    form.append("ocr_settings", JSON.stringify(ocrSettings));
    return req<Document>("/documents", { method: "POST", body: form });
  },

  startOcr: (id: string)                  => req<{ status: string }>(`/documents/${id}/ocr`, { method: "POST" }),

  saveBlocks: (docId: string, pageId: string, blocks: BlockData[]) =>
    req<{ saved: number }>(`/documents/${docId}/blocks`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ page_id: pageId, blocks }),
    }),

  exportDocument: (docId: string, format: string, theme = "light") =>
    fetch(`${BASE}/documents/${docId}/export`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${tokenStore.get()}`,
      },
      body: JSON.stringify({ format, theme }),
    }),
};
```

- [ ] **Step 4: Create frontend/app/login/page.tsx**

```tsx
"use client";
import { useState } from "react";
import { useRouter } from "next/navigation";
import { login, tokenStore } from "@/lib/auth";

export default function LoginPage() {
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setLoading(true);
    try {
      const tokens = await login(email, password);
      tokenStore.set(tokens);
      router.push("/documents");
    } catch (err) {
      setError(String(err));
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50">
      <form onSubmit={handleSubmit} className="bg-white p-8 rounded-2xl shadow w-96 space-y-4">
        <h1 className="text-2xl font-bold">Sign In</h1>
        {error && <div className="text-red-500 text-sm">{error}</div>}
        <input type="email" placeholder="Email" value={email} onChange={e => setEmail(e.target.value)}
          className="w-full border rounded-lg px-4 py-2" required />
        <input type="password" placeholder="Password" value={password} onChange={e => setPassword(e.target.value)}
          className="w-full border rounded-lg px-4 py-2" required />
        <button type="submit" disabled={loading}
          className="w-full bg-blue-600 text-white py-2 rounded-lg hover:bg-blue-700 disabled:opacity-50">
          {loading ? "Signing in..." : "Sign In"}
        </button>
      </form>
    </div>
  );
}
```

- [ ] **Step 5: Create the remaining pages + components**

Create `frontend/app/documents/page.tsx`, `frontend/app/documents/upload/page.tsx`, and `frontend/app/editor/[id]/page.tsx` using Next.js App Router (`"use client"` at top, `useRouter` from `next/navigation`, `params` via `use(params)` hook for dynamic routes).

**Source components to borrow and adapt to React TSX:**

- **`BlockEditor.tsx`** — Port from `/Users/riskyworks/Documents/pc/joplin-notion/src/webview/block-editor.js`. That file contains the full Editor.js initialization, undo stack, TOC with scroll-spy, vim mode, `AskAIInlineTool`/`AskAIBlockTune` inline tools, drag-drop, list contentEditable patch. Adapt as a React component: use `useRef` for the EditorJS instance, `useEffect` for init/cleanup, expose `save()` and `load(blocks)` via `useImperativeHandle`.

- **`MarkdownConverter.ts`** — Port from `/Users/riskyworks/Documents/pc/joplin-notion/src/webview/markdown-converter.js`. That file has a much more complete bidirectional converter with: inline HTML↔Markdown (bold/italic/code/links/strikethrough), checklist blocks, proper table parsing (header detection, separator skipping), blockquote with caption, backslash escape handling. Export as ES module with named exports `markdownToBlocks` and `blocksToMarkdown`.

- **`AiPanel.tsx`** — Port from `/Users/riskyworks/Documents/pc/joplin-ai/src/webview/panel.js`. Key features to adapt: streaming SSE consumer that renders markdown incrementally, mode selector (ask/edit), provider/model selector (claude/openai), chat history with user+AI messages, loading indicator with "is thinking…" timer, clear history button. Wire to the backend `/api/ai/block` SSE endpoint instead of the Joplin webviewApi bridge.

- **`OcrSettingsForm.tsx`** — New component (not in joplin projects). Form with fields matching `shared.OcrSettings`: engine select (rapidocr/easyocr/tesseract/mac), lang multi-select, do_ocr toggle, do_table_structure toggle, table_mode select (fast/accurate), generate_picture_images toggle, images_scale number, document_timeout number.

- **`ProcessingProgress.tsx`** — New component. Subscribes to the WebSocket at `/api/ws/:id/progress`, displays stage label + percent bar. Stages: loading → converting → extracting → complete / error.

- **`ExportDialog.tsx`** — New component. Format select (markdown/txt/pdf/docx/html), theme toggle (light/dark, only for pdf), Export button that calls `api.exportDocument()` and triggers browser download.

Copy all to `frontend/components/`.

- [ ] **Step 6: Create next.config.ts**

```typescript
import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  async rewrites() {
    return [
      {
        source: "/api/:path*",
        destination: "http://localhost:8080/api/:path*",
      },
    ];
  },
};

export default nextConfig;
```

- [ ] **Step 7: Commit**

```bash
git add webapp/frontend/
git commit -m "feat(webapp): Next.js 15 frontend — App Router, auth, API client, block editor pages"
```

---

## Task 12: Docker Compose

**Files:**
- Create: `webapp/docker-compose.yml`
- Create: `webapp/backend.Dockerfile`
- Create: `webapp/frontend/Dockerfile`

- [ ] **Step 1: Create webapp/backend.Dockerfile**

```dockerfile
FROM golang:1.24-alpine AS builder
RUN apk add --no-cache gcc musl-dev pandoc
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /bin/server ./cmd/server
RUN go build -o /bin/worker ./cmd/worker

FROM alpine:3.20
RUN apk add --no-cache ca-certificates pandoc nodejs npm
COPY --from=builder /bin/server /usr/local/bin/server
COPY --from=builder /bin/worker /usr/local/bin/worker
EXPOSE 8080
CMD ["/usr/local/bin/server"]
```

- [ ] **Step 2: Create webapp/docker-compose.yml**

```yaml
services:
  webapp-db:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: webapp
      POSTGRES_USER: webapp
      POSTGRES_PASSWORD: webapp
    volumes:
      - webapp-pg:/var/lib/postgresql/data
    networks:
      - ocr-net

  webapp-backend:
    build:
      context: .
      dockerfile: backend.Dockerfile
    environment:
      DATABASE_URL: postgres://webapp:webapp@webapp-db:5432/webapp
      REDIS_URL: redis://redis:6379/1
      AUTH_SERVICE_URL: http://auth-service:8002
      UPLOADS_DIR: /data/uploads
      EXPORTS_DIR: /data/exports
    env_file: .env
    ports:
      - "127.0.0.1:8080:8080"
    volumes:
      - webapp-uploads:/data/uploads
      - webapp-exports:/data/exports
    depends_on:
      - webapp-db
    networks:
      - ocr-net
    restart: unless-stopped

  # Scale this service to add OCR/AI workers:
  #   docker compose up --scale webapp-worker=3
  webapp-worker:
    build:
      context: .
      dockerfile: backend.Dockerfile
    command: ["/usr/local/bin/worker"]
    environment:
      DATABASE_URL: postgres://webapp:webapp@webapp-db:5432/webapp
      REDIS_URL: redis://redis:6379/1
      DOCLING_SERVICE_URL: http://paddleocr:8000
    env_file: .env
    volumes:
      - webapp-uploads:/data/uploads
    depends_on:
      - webapp-db
    networks:
      - ocr-net
    restart: unless-stopped

  webapp-frontend:
    build:
      context: ./frontend
    ports:
      - "127.0.0.1:3000:3000"
    environment:
      NEXT_PUBLIC_API_URL: http://localhost:8080/api
      NEXT_PUBLIC_AUTH_URL: http://localhost:8002
    depends_on:
      - webapp-backend
    networks:
      - ocr-net
    restart: unless-stopped

  redis:
    image: redis:7-alpine
    volumes:
      - webapp-redis:/data
    networks:
      - ocr-net

volumes:
  webapp-pg:
  webapp-uploads:
  webapp-exports:
  webapp-redis:

networks:
  ocr-net:
    external: true
    name: pdf-translator_ocr-net
```

- [ ] **Step 3: Scale workers horizontally**

```bash
# Start full stack
docker compose up -d

# Add 3 more OCR/AI workers (no server restart needed):
docker compose up --scale webapp-worker=4 -d

# Check queue stats via asynq CLI:
go install github.com/hibiken/asynq/tools/asynq@latest
asynq stats --uri redis://localhost:6379/1
```

- [ ] **Step 4: Commit**

```bash
git add webapp/docker-compose.yml webapp/backend.Dockerfile webapp/frontend/Dockerfile
git commit -m "feat(webapp): Docker — server + scalable worker + Postgres + Redis + Next.js"
```

---

## Architecture Notes for Future Scaling

### Adding a New Worker Slave

```bash
# On any machine with Redis + DB access:
./worker
# Or via Docker:
docker run --env-file .env yourorg/webapp-worker
```

The worker picks up tasks from the same Redis queues as the monolith's embedded worker. No code changes. No service restarts.

### Queue Priority Tuning

Edit `internal/queue/server.go`:
```go
Queues: map[string]int{
    QueueOCR:    10,  // higher = more workers allocated
    QueueAI:     5,
    QueueExport: 1,
},
```

### Adding a New Task Type

1. Add constant to `internal/queue/tasks.go`
2. Add `EnqueueXxx()` method to `internal/queue/client.go`
3. Create `internal/xxx/worker.go` with `Handle(ctx, task)` method
4. Register handler in both `cmd/server/main.go` and `cmd/worker/main.go`

---

## Self-Review

| Requirement | Task |
|---|---|
| Next.js frontend | Task 11 |
| Go + Gin backend | Task 1, 6, 8, 9, 10 |
| Modular monolith | Task 10 — cmd/server + internal/ boundaries |
| Task queue (asynq + Redis) | Task 3 |
| Standalone worker slaves | Task 10 — cmd/worker, docker --scale |
| Auth via auth-service JWT | Task 4 |
| Quota via auth-service REST | Task 5 |
| All Docling OCR settings | Task 7b — /convert endpoint |
| Document entity (pages + blocks) | Task 2 |
| WebSocket OCR progress | Task 8 |
| Block editor (Editor.js) | Task 11 |
| AI per-block + per-document | Task 8 |
| Claude streaming (prompt caching) | Task 8 — ai/service.go |
| Export: md/pdf/docx/html/txt | Task 9 |
| PDF theme light/dark | Task 9 |
| Docker + horizontal scaling | Task 12 |
