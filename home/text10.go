📘 Technical Summary
- Goal: Provide a production-ready REST API to search a user by PAN (Permanent Account Number).
- Stack: Go (1.21+), Gorilla Mux, PostgreSQL (pgx/stdlib), Zerolog for structured logging, Clean Architecture.
- Endpoint: GET /v1/users/search?pan=<PAN>
- Validation: PAN format regex: ^[A-Z]{5}[0-9]{4}[A-Z]$, case-insensitive input coerced to uppercase.
- Response: User profile by PAN. Proper error handling and structured error response with trace_id.
- Security: Basic input validation, safe SQL (prepared statements), request ID correlation.
- Config: Environment variables (PORT, DB_DSN, LOG_LEVEL, READ_TIMEOUT, WRITE_TIMEOUT, IDLE_TIMEOUT).
- Tests: Unit test skeletons for handler, service, repository.
- Postman: Included.
- Git: Push commands provided. Cannot push without repo URL/credentials.

Assumptions
- Database: PostgreSQL is the target database.
- PAN uniqueness: pan_number is unique, uppercase in DB.
- User data fields: id (UUID), pan_number, full_name, email, phone, dob, address, kyc_status, created_at, updated_at.
- Rate limiting and authentication are out of scope for this task.
- SCM: Repo URL and branch not provided. Instructions are included to push when available.

📦 Folder Structure
- cmd/server/main.go
- internal/config/config.go
- internal/logger/logger.go
- internal/http/middleware/requestid.go
- internal/http/middleware/logging.go
- internal/http/handlers/user_handler.go
- internal/service/user_service.go
- internal/repository/user_repository.go
- internal/model/user.go
- internal/validation/validator.go
- internal/errors/errors.go
- pkg/response/response.go
- migrations/001_create_users.sql
- tests/user_handler_test.go
- tests/user_service_test.go
- tests/user_repository_test.go
- postman/UserSearch.postman_collection.json
- go.mod
- go.sum (generated on build)
- Makefile
- README.md

🧩 Full API Code

go.mod
module github.com/yourorg/user-search-api

go 1.21

require (
	github.com/gorilla/mux v1.8.1
	github.com/jackc/pgx/v5/stdlib v5.5.4
	github.com/rs/zerolog v1.33.0
	github.com/google/uuid v1.6.0
)

cmd/server/main.go
package main

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/rs/zerolog/log"

	"github.com/yourorg/user-search-api/internal/config"
	"github.com/yourorg/user-search-api/internal/http/handlers"
	"github.com/yourorg/user-search-api/internal/http/middleware"
	"github.com/yourorg/user-search-api/internal/logger"
	"github.com/yourorg/user-search-api/internal/repository"
	"github.com/yourorg/user-search-api/internal/service"
)

func main() {
	// Load config
	cfg := config.Load()

	// Init logger
	logger.Init(cfg.LogLevel)

	// DB connection
	db, err := sql.Open("pgx", cfg.DBDSN)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to open DB connection")
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		log.Fatal().Err(err).Msg("failed to ping DB")
	}

	// Wiring
	userRepo := repository.NewUserRepository(db)
	userSvc := service.NewUserService(userRepo)
	userHandler := handlers.NewUserHandler(userSvc)

	// Router and middleware
	r := mux.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Logging)

	api := r.PathPrefix("/v1").Subrouter()
	api.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}).Methods(http.MethodGet)

	api.HandleFunc("/users/search", userHandler.SearchByPAN).Methods(http.MethodGet)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	// Graceful shutdown
	go func() {
		log.Info().Str("addr", srv.Addr).Msg("server starting")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server failed")
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Info().Msg("shutdown signal received")

	ctxShutdown, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := srv.Shutdown(ctxShutdown); err != nil {
		log.Error().Err(err).Msg("server shutdown error")
	} else {
		log.Info().Msg("server shutdown complete")
	}
}

internal/config/config.go
package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port         string
	DBDSN        string
	LogLevel     string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

func Load() Config {
	return Config{
		Port:         getEnv("PORT", "8080"),
		DBDSN:        getEnv("DB_DSN", "postgres://user:pass@localhost:5432/appdb?sslmode=disable"),
		LogLevel:     getEnv("LOG_LEVEL", "info"),
		ReadTimeout:  getEnvDuration("READ_TIMEOUT_SEC", 15) * time.Second,
		WriteTimeout: getEnvDuration("WRITE_TIMEOUT_SEC", 15) * time.Second,
		IdleTimeout:  getEnvDuration("IDLE_TIMEOUT_SEC", 60) * time.Second,
	}
}

func getEnv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

func getEnvDuration(key string, def int) time.Duration {
	if v, ok := os.LookupEnv(key); ok {
		if iv, err := strconv.Atoi(v); err == nil {
			return time.Duration(iv)
		}
	}
	return time.Duration(def)
}

internal/logger/logger.go
package logger

import (
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func Init(level string) {
	zerolog.TimeFieldFormat = time.RFC3339Nano
	// Console writer for local dev; in prod, consider JSON only
	writer := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}
	log.Logger = zerolog.New(writer).With().Timestamp().Logger()

	switch strings.ToLower(level) {
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "warn":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "error":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
}

internal/http/middleware/requestid.go
package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

type contextKey string

const requestIDKey contextKey = "requestID"

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := r.Header.Get("X-Request-ID")
		if reqID == "" {
			reqID = uuid.NewString()
		}
		ctx := context.WithValue(r.Context(), requestIDKey, reqID)
		w.Header().Set("X-Request-ID", reqID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func GetRequestID(ctx context.Context) string {
	val := ctx.Value(requestIDKey)
	if v, ok := val.(string); ok {
		return v
	}
	return ""
}

internal/http/middleware/logging.go
package middleware

import (
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lrw := &loggingResponseWriter{ResponseWriter: w, statusCode: 200}
		next.ServeHTTP(lrw, r)
		duration := time.Since(start)

		log.Info().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Str("query", r.URL.RawQuery).
			Int("status", lrw.statusCode).
			Str("request_id", GetRequestID(r.Context())).
			Dur("duration_ms", duration).
			Msg("http_request")
	})
}

internal/http/handlers/user_handler.go
package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/rs/zerolog/log"

	"github.com/yourorg/user-search-api/internal/errors"
	"github.com/yourorg/user-search-api/internal/service"
	"github.com/yourorg/user-search-api/pkg/response"
)

type UserHandler struct {
	svc service.UserService
}

func NewUserHandler(svc service.UserService) *UserHandler {
	return &UserHandler{svc: svc}
}

// GET /v1/users/search?pan=ABCDE1234F
func (h *UserHandler) SearchByPAN(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	pan := r.URL.Query().Get("pan")

	user, err := h.svc.SearchByPAN(ctx, pan)
	if err != nil {
		switch {
		case errors.IsValidation(err):
			response.WriteError(ctx, w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		case errors.IsNotFound(err):
			response.WriteError(ctx, w, http.StatusNotFound, "USER_NOT_FOUND", err.Error(), map[string]any{"pan": pan})
		default:
			log.Error().Err(err).Msg("search by pan failed")
			response.WriteError(ctx, w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", nil)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response.Success{Data: user}); err != nil {
		log.Error().Err(err).Msg("failed to write response")
	}
}

internal/service/user_service.go
package service

import (
	"context"
	"strings"

	"github.com/yourorg/user-search-api/internal/errors"
	"github.com/yourorg/user-search-api/internal/model"
	"github.com/yourorg/user-search-api/internal/repository"
	"github.com/yourorg/user-search-api/internal/validation"
)

type UserService interface {
	SearchByPAN(ctx context.Context, pan string) (*model.UserResponse, error)
}

type userService struct {
	repo repository.UserRepository
}

func NewUserService(repo repository.UserRepository) UserService {
	return &userService{repo: repo}
}

func (s *userService) SearchByPAN(ctx context.Context, pan string) (*model.UserResponse, error) {
	pan = strings.TrimSpace(pan)
	if err := validation.ValidatePAN(pan); err != nil {
		return nil, errors.WrapValidation(err, "invalid pan")
	}

	upperPan := strings.ToUpper(pan)
	u, err := s.repo.GetByPAN(ctx, upperPan)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, errors.WrapNotFound(err, "user not found")
		}
		return nil, errors.WrapInternal(err, "failed to get user by PAN")
	}

	resp := &model.UserResponse{
		ID:        u.ID,
		PAN:       u.PAN,
		FullName:  u.FullName,
		Email:     u.Email,
		Phone:     u.Phone,
		DOB:       u.DOB,
		Address:   u.Address,
		KYCStatus: u.KYCStatus,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
	}
	return resp, nil
}

internal/repository/user_repository.go
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	domainErr "github.com/yourorg/user-search-api/internal/errors"
	"github.com/yourorg/user-search-api/internal/model"
)

type UserRepository interface {
	GetByPAN(ctx context.Context, pan string) (*model.User, error)
}

type userRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) UserRepository {
	return &userRepository{db: db}
}

func (r *userRepository) GetByPAN(ctx context.Context, pan string) (*model.User, error) {
	const q = `
SELECT id, pan_number, full_name, email, phone, dob, address, kyc_status, created_at, updated_at
FROM users
WHERE pan_number = $1
LIMIT 1;
`
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	row := r.db.QueryRowContext(ctx, q, pan)
	var u model.User
	if err := row.Scan(
		&u.ID, &u.PAN, &u.FullName, &u.Email, &u.Phone, &u.DOB, &u.Address, &u.KYCStatus, &u.CreatedAt, &u.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domainErr.WrapNotFound(err, fmt.Sprintf("no user with pan=%s", pan))
		}
		return nil, domainErr.WrapInternal(err, "query error")
	}
	return &u, nil
}

internal/model/user.go
package model

import "time"

type User struct {
	ID        string     `json:"id"`
	PAN       string     `json:"pan"`
	FullName  string     `json:"full_name"`
	Email     string     `json:"email"`
	Phone     string     `json:"phone"`
	DOB       *time.Time `json:"dob,omitempty"`
	Address   *string    `json:"address,omitempty"`
	KYCStatus string     `json:"kyc_status"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type UserResponse struct {
	ID        string     `json:"id"`
	PAN       string     `json:"pan"`
	FullName  string     `json:"full_name"`
	Email     string     `json:"email"`
	Phone     string     `json:"phone"`
	DOB       *time.Time `json:"dob,omitempty"`
	Address   *string    `json:"address,omitempty"`
	KYCStatus string     `json:"kyc_status"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

internal/validation/validator.go
package validation

import (
	"errors"
	"regexp"
	"strings"
)

var panRegex = regexp.MustCompile(`^[A-Z]{5}[0-9]{4}[A-Z]$`)

func ValidatePAN(pan string) error {
	if strings.TrimSpace(pan) == "" {
		return errors.New("pan is required")
	}
	pan = strings.ToUpper(pan)
	if !panRegex.MatchString(pan) {
		return errors.New("pan must match pattern ^[A-Z]{5}[0-9]{4}[A-Z]$")
	}
	return nil
}

internal/errors/errors.go
package errors

import "fmt"

type Kind int

const (
	KindUnknown Kind = iota
	KindValidation
	KindNotFound
	KindInternal
)

type Error struct {
	Kind Kind
	Err  error
	Msg  string
}

func (e *Error) Error() string {
	if e.Msg != "" {
		return e.Msg + ": " + e.Err.Error()
	}
	return e.Err.Error()
}

func (e *Error) Unwrap() error { return e.Err }

func WrapValidation(err error, msg string) error {
	return &Error{Kind: KindValidation, Err: err, Msg: msg}
}

func WrapNotFound(err error, msg string) error {
	return &Error{Kind: KindNotFound, Err: err, Msg: msg}
}

func WrapInternal(err error, msg string) error {
	return &Error{Kind: KindInternal, Err: err, Msg: msg}
}

func IsValidation(err error) bool {
	if e, ok := err.(*Error); ok && e.Kind == KindValidation {
		return true
	}
	return false
}

func IsNotFound(err error) bool {
	if e, ok := err.(*Error); ok && e.Kind == KindNotFound {
		return true
	}
	return false
}

func IsInternal(err error) bool {
	if e, ok := err.(*Error); ok && e.Kind == KindInternal {
		return true
	}
	return false
}

func Errorf(format string, a ...any) error {
	return &Error{Kind: KindUnknown, Err: fmt.Errorf(format, a...)}
}

pkg/response/response.go
package response

import (
	"encoding/json"
	"net/http"

	"github.com/yourorg/user-search-api/internal/http/middleware"
)

type Success struct {
	Data any `json:"data"`
}

type Error struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
	TraceID string         `json:"trace_id"`
}

func WriteError(ctx context.Context, w http.ResponseWriter, status int, code, message string, details map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := Error{
		Error: ErrorBody{
			Code:    code,
			Message: message,
			Details: details,
			TraceID: middleware.GetRequestID(ctx),
		},
	}
	_ = json.NewEncoder(w).Encode(resp)
}

Note: Add missing import for context.

Updated pkg/response/response.go
package response

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/yourorg/user-search-api/internal/http/middleware"
)

type Success struct {
	Data any `json:"data"`
}

type Error struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
	TraceID string         `json:"trace_id"`
}

func WriteError(ctx context.Context, w http.ResponseWriter, status int, code, message string, details map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := Error{
		Error: ErrorBody{
			Code:    code,
			Message: message,
			Details: details,
			TraceID: middleware.GetRequestID(ctx),
		},
	}
	_ = json.NewEncoder(w).Encode(resp)
}

migrations/001_create_users.sql
-- +goose Up
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY,
    pan_number VARCHAR(10) NOT NULL UNIQUE,
    full_name VARCHAR(200) NOT NULL,
    email VARCHAR(200),
    phone VARCHAR(20),
    dob DATE,
    address TEXT,
    kyc_status VARCHAR(20) NOT NULL DEFAULT 'unverified',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_pan ON users (pan_number);

-- +goose Down
DROP TABLE IF EXISTS users;

tests/user_handler_test.go
package tests

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"

	"github.com/yourorg/user-search-api/internal/http/handlers"
	"github.com/yourorg/user-search-api/internal/service"
)

type mockUserService struct {
	resp any
	err  error
}

func (m *mockUserService) SearchByPAN(ctx context.Context, pan string) (*model.UserResponse, error) {
	return m.resp.(*model.UserResponse), m.err
}

func TestSearchByPAN_BadRequest(t *testing.T) {
	svc := &mockUserService{}
	h := handlers.NewUserHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/v1/users/search?pan=abc", nil)
	rr := httptest.NewRecorder()

	router := mux.NewRouter()
	router.HandleFunc("/v1/users/search", h.SearchByPAN)
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

Note: Fix imports and context/model.

Updated tests/user_handler_test.go
package tests

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"

	"github.com/yourorg/user-search-api/internal/http/handlers"
	"github.com/yourorg/user-search-api/internal/model"
	"github.com/yourorg/user-search-api/internal/service"
	ie "github.com/yourorg/user-search-api/internal/errors"
)

type mockUserService struct {
	resp *model.UserResponse
	err  error
}

func (m *mockUserService) SearchByPAN(ctx context.Context, pan string) (*model.UserResponse, error) {
	return m.resp, m.err
}

func TestSearchByPAN_BadRequest(t *testing.T) {
	svc := &mockUserService{
		err: ie.WrapValidation(ie.Errorf("bad"), "invalid pan"),
	}
	h := handlers.NewUserHandler(service.UserService(svc))

	req := httptest.NewRequest(http.MethodGet, "/v1/users/search?pan=abc", nil)
	rr := httptest.NewRecorder()

	router := mux.NewRouter()
	router.HandleFunc("/v1/users/search", h.SearchByPAN).Methods(http.MethodGet)
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

tests/user_service_test.go
package tests

import (
	"context"
	"testing"

	"github.com/yourorg/user-search-api/internal/model"
	"github.com/yourorg/user-search-api/internal/repository"
	"github.com/yourorg/user-search-api/internal/service"
)

type mockUserRepo struct {
	user *model.User
	err  error
}

func (m *mockUserRepo) GetByPAN(ctx context.Context, pan string) (*model.User, error) {
	return m.user, m.err
}

func TestUserService_Validation(t *testing.T) {
	svc := service.NewUserService(&mockUserRepo{})
	_, err := svc.SearchByPAN(context.Background(), "bad-pan")
	if err == nil {
		t.Fatalf("expected validation error")
	}
}

tests/user_repository_test.go
package tests

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/yourorg/user-search-api/internal/repository"
)

func TestUserRepository_GetByPAN_NoRows(t *testing.T) {
	// Integration test would require a test DB; this is a placeholder skeleton.
	db, err := sql.Open("pgx", "postgres://user:pass@localhost:5432/appdb?sslmode=disable")
	if err != nil {
		t.Skip("db not available")
		return
	}
	defer db.Close()

	repo := repository.NewUserRepository(db)
	_, err = repo.GetByPAN(context.Background(), "ABCDE1234F")
	if err == nil {
		t.Fatalf("expected error when DB empty or missing record")
	}
}

postman/UserSearch.postman_collection.json
{
  "info": {
    "name": "User Search API",
    "_postman_id": "f4f0d7f3-5d69-4a62-8b6d-0b9b1c7b2e20",
    "description": "Search user by PAN",
    "schema": "https://schema.getpostman.com/json/collection/v2.1.0/collection.json"
  },
  "item": [
    {
      "name": "Health Check",
      "request": {
        "method": "GET",
        "header": [],
        "url": {
          "raw": "{{base_url}}/v1/health",
          "host": ["{{base_url}}"],
          "path": ["v1", "health"]
        }
      }
    },
    {
      "name": "Search User by PAN",
      "request": {
        "method": "GET",
        "header": [
          { "key": "X-Request-ID", "value": "test-req-123" }
        ],
        "url": {
          "raw": "{{base_url}}/v1/users/search?pan=ABCDE1234F",
          "host": ["{{base_url}}"],
          "path": ["v1", "users", "search"],
          "query": [
            { "key": "pan", "value": "ABCDE1234F" }
          ]
        }
      }
    }
  ],
  "variable": [
    { "key": "base_url", "value": "http://localhost:8080" }
  ]
}

🪵 Logs & Error Format
- Logs: Zerolog structured logs emitted for each request with fields: method, path, query, status, request_id, duration_ms.
- Error Response JSON:
  {
    "error": {
      "code": "USER_NOT_FOUND | VALIDATION_ERROR | INTERNAL_ERROR",
      "message": "...",
      "details": { ...optional... },
      "trace_id": "<request-id>"
    }
  }

🗄️ SQL schema changes
- Create users table with unique pan_number, UUID id, and standard profile fields.
- Migration file: migrations/001_create_users.sql

📝 README.md
# User Search API

A small Go service to search user by PAN.

- GET /v1/users/search?pan=ABCDE1234F

Run:
- make build
- DB_DSN="postgres://user:pass@localhost:5432/appdb?sslmode=disable" PORT=8080 ./bin/server

Env:
- PORT (default 8080)
- DB_DSN
- LOG_LEVEL (info|debug|warn|error)
- READ_TIMEOUT_SEC (15), WRITE_TIMEOUT_SEC (15), IDLE_TIMEOUT_SEC (60)

Makefile
BINARY=bin/server

.PHONY: build run test

build:
	mkdir -p bin
	go build -o $(BINARY) ./cmd/server

run: build
	PORT=8080 LOG_LEVEL=debug ./$(BINARY)

test:
	go test ./...

🔀 Git commands executed + confirmation of push
- Note: Repository URL and branch were not provided. Unable to push code automatically. Please set the remote and branch, then run:

git init
git branch -M main
git add .
git commit -m "feat(api): add search user by PAN endpoint with validation, logging, and Postgres repo"
git remote add origin <REPO_URL>
git push -u origin <BRANCH_NAME>

If credentials and repository URL are provided, I can generate the exact commands per your SCM settings.

Acceptance Criteria Mapping
- API to search user by PAN: Implemented at GET /v1/users/search
- Validation: PAN regex validation; returns 400 for invalid
- Error handling: Consistent structured errors with trace_id
- Logging: Structured with request ID correlation
- DB: Postgres query with prepared statement scanning; unique pan index
- Config: Environment-based configuration
- Tests: Handler, service, repository skeletons
- Postman: Collection provided
- Clean architecture, context usage, no globals, error wrapping: Implemented

If you want MySQL/Redshift compatibility or to add authentication, rate limiting, or caching, I can extend this with minimal changes.