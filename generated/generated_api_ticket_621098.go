Below is the complete deliverable as per the BRD and project constraints.

1. Folder Structure
- internal/recommendedsocialaccount/
  - controller/
    - controller.go
  - service/
    - service.go
  - repository/
    - repository.go
  - model/
    - model.go
  - dto/
    - request.go
    - response.go
  - validator/
    - validator.go
  - sql/
    - queries.sql
  - tests/
    - service_test.go

2. Model (structs matching table columns exactly)
// internal/recommendedsocialaccount/model/model.go
package models

import "time"

type RecommendedSocialAccount struct {
	RecommendedSocialAccountID  int64     `json:"recommended_social_account_id"`
	FkAffiliateSocialAccountID  int64     `json:"fk_affiliate_social_account_id"`
	RecommendedSocialAccountURL string    `json:"recommended_social_account_url"`
	RecommendedSocialAccountDesc string   `json:"recommended_social_account_desc"`
	IsOnboarded                 int       `json:"is_onboarded"`
	InsertDate                  time.Time `json:"insert_date"`
	LastUpdateDate              time.Time `json:"last_update_date"`
}

3. DTOs (Request + Response)
// internal/recommendedsocialaccount/dto/request.go
package dto

import "time"

type RecommendedSocialAccountInsertItem struct {
	FkAffiliateSocialAccountID   int64      `json:"fk_affiliate_social_account_id"`
	RecommendedSocialAccountURL  string     `json:"recommended_social_account_url"`
	RecommendedSocialAccountDesc string     `json:"recommended_social_account_desc"`
	IsOnboarded                  int        `json:"is_onboarded"`
	InsertDate                   *time.Time `json:"insert_date,omitempty"`
	LastUpdateDate               *time.Time `json:"last_update_date,omitempty"`
}

type BulkInsertRequest struct {
	Source string                             `json:"source,omitempty"` // For audit logging
	Items  []RecommendedSocialAccountInsertItem `json:"items"`
}

type ReadRequest struct {
	FkAffiliateSocialAccountIDs []int64 `json:"fk_affiliate_social_account_ids"`
}

// internal/recommendedsocialaccount/dto/response.go
package dto

import "time"

type APIError struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

type BulkInsertResponse struct {
	InsertedIDs []int64 `json:"inserted_ids"`
	Count       int     `json:"count"`
}

type RecommendedChannel struct {
	RecommendedSocialAccountID  int64     `json:"recommended_social_account_id"`
	RecommendedSocialAccountURL string    `json:"recommended_social_account_url"`
	RecommendedSocialAccountDesc string   `json:"recommended_social_account_desc"`
	IsOnboarded                 int       `json:"is_onboarded"`
	InsertDate                  time.Time `json:"insert_date"`
	LastUpdateDate              time.Time `json:"last_update_date"`
}

type GroupedRecommendedChannels struct {
	FkAffiliateSocialAccountID int64                 `json:"fk_affiliate_social_account_id"`
	Channels                   []RecommendedChannel  `json:"channels"`
}

type ReadResponse struct {
	Data  []GroupedRecommendedChannels `json:"data"`
	Count int                          `json:"count"`
}

4. Controller code
// internal/recommendedsocialaccount/controller/controller.go
package controller

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"internal/recommendedsocialaccount/dto"
	"internal/recommendedsocialaccount/service"

	// logging functions are available as logs.Info/logs.Error as per existing project functions
)

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code, message string, details interface{}) {
	resp := dto.APIError{
		Code:    code,
		Message: message,
		Details: details,
	}
	writeJSON(w, status, resp)
}

// POST /api/v1/recommended-social-accounts/bulk
func BulkInsertRecommendedSocialAccountsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req dto.BulkInsertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logs.Error("Bulk insert parse error", err, map[string]interface{}{"path": r.URL.Path})
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON payload", nil)
		return
	}

	// Log source and metadata for audit
	logs.Info("Bulk insert recommended social accounts request received",
		map[string]interface{}{
			"path":   r.URL.Path,
			"source": req.Source,
			"count":  len(req.Items),
		})

	resp, apiErr := service.BulkInsertRecommendedSocialAccounts(ctx, req)
	if apiErr != nil {
		status := http.StatusInternalServerError
		switch apiErr.Code {
		case "BAD_REQUEST":
			status = http.StatusBadRequest
		case "INVALID_AFFILIATE_SOCIAL_ACCOUNT_ID":
			status = http.StatusBadRequest
		case "MAX_LIMIT_EXCEEDED":
			status = http.StatusBadRequest
		case "CONFLICT":
			status = http.StatusConflict
		default:
			// keep 500
		}
		writeError(w, status, apiErr.Code, apiErr.Message, apiErr.Details)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// GET /api/v1/recommended-social-accounts?fk_affiliate_social_account_id=1,2,3
func ReadRecommendedSocialAccountsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	raw := r.URL.Query().Get("fk_affiliate_social_account_id")
	if raw == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "fk_affiliate_social_account_id is required", nil)
		return
	}
	parts := strings.Split(raw, ",")
	var ids []int64
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		id, err := strconv.ParseInt(p, 10, 64)
		if err != nil || id <= 0 {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid fk_affiliate_social_account_id list", map[string]interface{}{"value": p})
			return
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "fk_affiliate_social_account_id is required", nil)
		return
	}

	res, apiErr := service.ReadRecommendedSocialAccounts(ctx, dto.ReadRequest{FkAffiliateSocialAccountIDs: ids})
	if apiErr != nil {
		status := http.StatusInternalServerError
		if apiErr.Code == "NOT_FOUND" {
			status = http.StatusNotFound
		} else if apiErr.Code == "BAD_REQUEST" {
			status = http.StatusBadRequest
		}
		writeError(w, status, apiErr.Code, apiErr.Message, apiErr.Details)
		return
	}

	writeJSON(w, http.StatusOK, res)
}

5. Service layer code
// internal/recommendedsocialaccount/service/service.go
package service

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"internal/recommendedsocialaccount/dto"
	"internal/recommendedsocialaccount/repository"
	"internal/recommendedsocialaccount/validator"
)

type APIError = dto.APIError

func BulkInsertRecommendedSocialAccounts(ctx context.Context, req dto.BulkInsertRequest) (*dto.BulkInsertResponse, *APIError) {
	// Max rows check
	if len(req.Items) == 0 {
		return nil, &APIError{Code: "BAD_REQUEST", Message: "items is required and cannot be empty"}
	}
	if len(req.Items) > 25 {
		return nil, &APIError{Code: "MAX_LIMIT_EXCEEDED", Message: "maximum 25 items allowed"}
	}

	// Validate fields
	now := time.Now()
	for idx, item := range req.Items {
		if item.FkAffiliateSocialAccountID <= 0 {
			return nil, &APIError{Code: "BAD_REQUEST", Message: "fk_affiliate_social_account_id must be > 0", Details: map[string]interface{}{"index": idx}}
		}
		if strings.TrimSpace(item.RecommendedSocialAccountURL) == "" || !isValidHTTPURL(item.RecommendedSocialAccountURL) {
			return nil, &APIError{Code: "BAD_REQUEST", Message: "invalid URL (must be http/https)", Details: map[string]interface{}{"index": idx}}
		}
		if len(item.RecommendedSocialAccountURL) > 500 {
			return nil, &APIError{Code: "BAD_REQUEST", Message: "url length exceeds 500", Details: map[string]interface{}{"index": idx}}
		}
		if strings.TrimSpace(item.RecommendedSocialAccountDesc) == "" {
			return nil, &APIError{Code: "BAD_REQUEST", Message: "recommended_social_account_desc is required", Details: map[string]interface{}{"index": idx}}
		}
		if len(item.RecommendedSocialAccountDesc) > 1000 {
			return nil, &APIError{Code: "BAD_REQUEST", Message: "desc length exceeds 1000", Details: map[string]interface{}{"index": idx}}
		}
		if item.IsOnboarded != 0 && item.IsOnboarded != 1 {
			return nil, &APIError{Code: "BAD_REQUEST", Message: "is_onboarded must be 0 or 1", Details: map[string]interface{}{"index": idx}}
		}
		if item.InsertDate == nil {
			req.Items[idx].InsertDate = &now
		}
		if item.LastUpdateDate == nil {
			req.Items[idx].LastUpdateDate = &now
		}
	}

	// Validate affiliate social account IDs exist
	uniqueAffiliateIDs := uniqueAffiliateIDsFromItems(req.Items)
	missing, err := repository.GetMissingAffiliateSocialAccountIDs(ctx, uniqueAffiliateIDs)
	if err != nil {
		logs.Error("Validate affiliate social account IDs - DB error", err, map[string]interface{}{"ids": uniqueAffiliateIDs})
		return nil, &APIError{Code: "INTERNAL_ERROR", Message: "database error"}
	}
	if len(missing) > 0 {
		return nil, &APIError{
			Code:    "INVALID_AFFILIATE_SOCIAL_ACCOUNT_ID",
			Message: "one or more affiliate social account IDs are invalid",
			Details: map[string]interface{}{"missing_ids": missing},
		}
	}

	- Check duplicate URLs within payload per affiliate
	if dupIdx, dupURL, dupFk := findInPayloadDuplicates(req.Items); dupIdx >= 0 {
		return nil, &APIError{
			Code:    "CONFLICT",
			Message: "duplicate URL for same affiliate social account in request payload",
			Details: map[string]interface{}{"index": dupIdx, "url": dupURL, "fk_affiliate_social_account_id": dupFk},
		}
	}

	// Check duplicates with existing data (business restriction)
	existingDupMap, err := repository.FindExistingDuplicateURLs(ctx, req.Items)
	if err != nil {
		logs.Error("Check existing duplicates - DB error", err, nil)
		return nil, &APIError{Code: "INTERNAL_ERROR", Message: "database error"}
	}
	if len(existingDupMap) > 0 {
		return nil, &APIError{
			Code:    "CONFLICT",
			Message: "duplicate URL exists for same affiliate social account",
			Details: existingDupMap, // map[fk]list-of-urls
		}
	}

	// Perform atomic bulk insert
	ids, err := repository.BulkInsertRecommendedSocialAccounts(ctx, req.Items)
	if err != nil {
		logs.Error("Bulk insert recommended social accounts failed", err, map[string]interface{}{"count": len(req.Items)})
		return nil, &APIError{Code: "INTERNAL_ERROR", Message: "database error"}
	}

	logs.Info("Bulk insert recommended social accounts success",
		map[string]interface{}{"inserted_count": len(ids)})

	return &dto.BulkInsertResponse{
		InsertedIDs: ids,
		Count:       len(ids),
	}, nil
}

func ReadRecommendedSocialAccounts(ctx context.Context, req dto.ReadRequest) (*dto.ReadResponse, *APIError) {
	if len(req.FkAffiliateSocialAccountIDs) == 0 {
		return nil, &APIError{Code: "BAD_REQUEST", Message: "at least one fk_affiliate_social_account_id required"}
	}
	if len(req.FkAffiliateSocialAccountIDs) > 1000 {
		return nil, &APIError{Code: "BAD_REQUEST", Message: "too many IDs (max 1000)"}
	}

	records, err := repository.ReadByAffiliateSocialAccountIDs(ctx, req.FkAffiliateSocialAccountIDs)
	if err != nil {
		logs.Error("Read recommended social accounts - DB error", err, map[string]interface{}{"ids": req.FkAffiliateSocialAccountIDs})
		return nil, &APIError{Code: "INTERNAL_ERROR", Message: "database error"}
	}
	if len(records) == 0 {
		return nil, &APIError{Code: "NOT_FOUND", Message: "no recommended channels found for given affiliate IDs"}
	}

	grouped := make(map[int64][]dto.RecommendedChannel)
	for _, rec := range records {
		grouped[rec.FkAffiliateSocialAccountID] = append(grouped[rec.FkAffiliateSocialAccountID], dto.RecommendedChannel{
			RecommendedSocialAccountID:  rec.RecommendedSocialAccountID,
			RecommendedSocialAccountURL: rec.RecommendedSocialAccountURL,
			RecommendedSocialAccountDesc: rec.RecommendedSocialAccountDesc,
			IsOnboarded:                 rec.IsOnboarded,
			InsertDate:                  rec.InsertDate,
			LastUpdateDate:              rec.LastUpdateDate,
		})
	}

	var resp dto.ReadResponse
	for _, id := range req.FkAffiliateSocialAccountIDs {
		if channels, ok := grouped[id]; ok {
			resp.Data = append(resp.Data, dto.GroupedRecommendedChannels{
				FkAffiliateSocialAccountID: id,
				Channels:                   channels,
			})
		}
	}
	resp.Count = len(records)

	return &resp, nil
}

func isValidHTTPURL(s string) bool {
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	if u.Host == "" {
		return false
	}
	return true
}

func uniqueAffiliateIDsFromItems(items []dto.RecommendedSocialAccountInsertItem) []int64 {
	m := make(map[int64]struct{})
	for _, it := range items {
		m[it.FkAffiliateSocialAccountID] = struct{}{}
	}
	out := make([]int64, 0, len(m))
	for id := range m {
		out = append(out, id)
	}
	return out
}

func findInPayloadDuplicates(items []dto.RecommendedSocialAccountInsertItem) (idx int, url string, fk int64) {
	seen := make(map[int64]map[string]struct{})
	for i, it := range items {
		u := strings.TrimSpace(strings.ToLower(it.RecommendedSocialAccountURL))
		if _, ok := seen[it.FkAffiliateSocialAccountID]; !ok {
			seen[it.FkAffiliateSocialAccountID] = make(map[string]struct{})
		}
		if _, ok := seen[it.FkAffiliateSocialAccountID][u]; ok {
			return i, it.RecommendedSocialAccountURL, it.FkAffiliateSocialAccountID
		}
		seen[it.FkAffiliateSocialAccountID][u] = struct{}{}
	}
	return -1, "", 0
}

6. Repository layer code
// internal/recommendedsocialaccount/repository/repository.go
package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"internal/recommendedsocialaccount/dto"
	"internal/recommendedsocialaccount/models"
)

func placeholderList(n int) string {
	// Builds "?, ?, ?, ..."
	sb := strings.Builder{}
	for i := 0; i < n; i++ {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString("?")
	}
	return sb.String()
}

func GetMissingAffiliateSocialAccountIDs(ctx context.Context, ids []int64) ([]int64, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	q := fmt.Sprintf("SELECT affiliate_social_account_id FROM affiliate_social_account WHERE affiliate_social_account_id IN (%s)",
		placeholderList(len(ids)))
	args := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := ExecuteQuery(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	found := make(map[int64]struct{})
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		found[id] = struct{}{}
	}
	missing := make([]int64, 0)
	for _, id := range ids {
		if _, ok := found[id]; !ok {
			missing = append(missing, id)
		}
	}
	return missing, nil
}

// existing duplicate check per affiliate social account
func FindExistingDuplicateURLs(ctx context.Context, items []dto.RecommendedSocialAccountInsertItem) (map[int64][]string, error) {
	// group urls by fk id
	group := make(map[int64][]string)
	for _, it := range items {
		group[it.FkAffiliateSocialAccountID] = append(group[it.FkAffiliateSocialAccountID], strings.TrimSpace(strings.ToLower(it.RecommendedSocialAccountURL)))
	}
	result := make(map[int64][]string)

	for fk, urls := range group {
		uniq := make(map[string]struct{})
		var urlList []string
		for _, u := range urls {
			if _, ok := uniq[u]; !ok {
				uniq[u] = struct{}{}
				urlList = append(urlList, u)
			}
		}
		if len(urlList) == 0 {
			continue
		}
		q := fmt.Sprintf(`
			SELECT LOWER(recommended_social_account_url)
			FROM recommended_social_account
			WHERE fk_affiliate_social_account_id = ?
			  AND LOWER(recommended_social_account_url) IN (%s)
		`, placeholderList(len(urlList)))
		args := make([]interface{}, 0, len(urlList)+1)
		args = append(args, fk)
		for _, u := range urlList {
			args = append(args, u)
		}
		rows, err := ExecuteQuery(ctx, q, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var dups []string
		for rows.Next() {
			var u string
			if err := rows.Scan(&u); err != nil {
				return nil, err
			}
			dups = append(dups, u)
		}
		if len(dups) > 0 {
			result[fk] = dups
		}
	}
	return result, nil
}

// Atomic bulk insert of items; returns inserted IDs
func BulkInsertRecommendedSocialAccounts(ctx context.Context, items []dto.RecommendedSocialAccountInsertItem) ([]int64, error) {
	// Begin transaction (uses project ExecuteSQL helper)
	if _, err := ExecuteSQL(ctx, "BEGIN"); err != nil {
		return nil, err
	}

	insertSQL := `
		INSERT INTO recommended_social_account (
			fk_affiliate_social_account_id,
			recommended_social_account_url,
			recommended_social_account_desc,
			is_onboarded,
			insert_date,
			last_update_date
		)
		VALUES (?, ?, ?, ?, COALESCE(?, NOW()), COALESCE(?, NOW()))
		RETURNING recommended_social_account_id
	`

	var ids []int64
	for _, it := range items {
		args := []interface{}{
			it.FkAffiliateSocialAccountID,
			it.RecommendedSocialAccountURL,
			it.RecommendedSocialAccountDesc,
			it.IsOnboarded,
			sql.NullTime{Time: derefTime(it.InsertDate), Valid: it.InsertDate != nil},
			sql.NullTime{Time: derefTime(it.LastUpdateDate), Valid: it.LastUpdateDate != nil},
		}
		rows, err := ExecuteQuery(ctx, insertSQL, args...)
		if err != nil {
			_, _ = ExecuteSQL(ctx, "ROLLBACK")
			return nil, err
		}
		if rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				_, _ = ExecuteSQL(ctx, "ROLLBACK")
				return nil, err
			}
			ids = append(ids, id)
		}
		rows.Close()
	}

	if _, err := ExecuteSQL(ctx, "COMMIT"); err != nil {
		_, _ = ExecuteSQL(ctx, "ROLLBACK")
		return nil, err
	}
	return ids, nil
}

func derefTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

func ReadByAffiliateSocialAccountIDs(ctx context.Context, ids []int64) ([]models.RecommendedSocialAccount, error) {
	q := fmt.Sprintf(`
		SELECT
			recommended_social_account_id,
			fk_affiliate_social_account_id,
			recommended_social_account_url,
			recommended_social_account_desc,
			is_onboarded,
			insert_date,
			last_update_date
		FROM recommended_social_account
		WHERE fk_affiliate_social_account_id IN (%s)
		ORDER BY fk_affiliate_social_account_id, recommended_social_account_id
	`, placeholderList(len(ids)))

	args := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}

	rows, err := ExecuteQuery(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.RecommendedSocialAccount
	for rows.Next() {
		var m models.RecommendedSocialAccount
		if err := rows.Scan(
			&m.RecommendedSocialAccountID,
			&m.FkAffiliateSocialAccountID,
			&m.RecommendedSocialAccountURL,
			&m.RecommendedSocialAccountDesc,
			&m.IsOnboarded,
			&m.InsertDate,
			&m.LastUpdateDate,
		); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

7. SQL queries (INSERT + SELECT)
// internal/recommendedsocialaccount/sql/queries.sql
-- Bulk Insert (per row, within transaction)
INSERT INTO recommended_social_account (
  fk_affiliate_social_account_id,
  recommended_social_account_url,
  recommended_social_account_desc,
  is_onboarded,
  insert_date,
  last_update_date
)
VALUES (?, ?, ?, ?, COALESCE(?, NOW()), COALESCE(?, NOW()))
RETURNING recommended_social_account_id;

-- Read with IN filter
SELECT
  recommended_social_account_id,
  fk_affiliate_social_account_id,
  recommended_social_account_url,
  recommended_social_account_desc,
  is_onboarded,
  insert_date,
  last_update_date
FROM recommended_social_account
WHERE fk_affiliate_social_account_id IN (?, ?, ...)
ORDER BY fk_affiliate_social_account_id, recommended_social_account_id;

-- Duplicate check
SELECT LOWER(recommended_social_account_url)
FROM recommended_social_account
WHERE fk_affiliate_social_account_id = ?
  AND LOWER(recommended_social_account_url) IN (?, ?, ...);

-- Index recommendation for faster reads
CREATE INDEX IF NOT EXISTS idx_recommended_social_acc_fk
  ON recommended_social_account (fk_affiliate_social_account_id);

8. Request validation rules
- Bulk Insert:
  - items array is required and must contain 1 to 25 records; else MAX_LIMIT_EXCEEDED
  - All fields mandatory:
    - fk_affiliate_social_account_id > 0 and must exist in affiliate_social_account table
    - recommended_social_account_url must be a valid http/https URL; max length 500
    - recommended_social_account_desc required; max length 1000
    - is_onboarded must be 0 or 1
    - insert_date and last_update_date auto-populated to current time if omitted
  - Duplicates:
    - No duplicate URL per fk_affiliate_social_account_id within the same payload (409 CONFLICT)
    - No duplicate URL per fk_affiliate_social_account_id against existing data (409 CONFLICT)
  - Complete atomic insert: If any record fails → entire batch fails; no partial insert

- Read:
  - fk_affiliate_social_account_id must be provided as CSV in query param; all must be positive integers
  - Accept multiple values; no pagination

9. Logging using existing project functions
- Controller logs incoming POST requests with source and item count
- Service logs at key steps and on errors
- Repository does not log directly (service handles errors/logs)
- Use:
  - logs.Info("message", fieldsMap)
  - logs.Error("message", err, fieldsMap)

10. DB operations using existing functions
- db := GetDB(ctx) is available (used implicitly; we rely on ExecuteSQL/ExecuteQuery)
- ExecuteSQL(ctx, query, args...)
- ExecuteQuery(ctx, query, args...)

All repository functions use only ExecuteSQL/ExecuteQuery with parameterized SQL.

11. Error handling using existing central error format
- APIError struct:
  - code: BAD_REQUEST, INVALID_AFFILIATE_SOCIAL_ACCOUNT_ID, MAX_LIMIT_EXCEEDED, CONFLICT, NOT_FOUND, INTERNAL_ERROR
  - message: human readable
  - details: index of failed row or missing IDs or duplicates map

HTTP code mapping:
- 400: BAD_REQUEST, INVALID_AFFILIATE_SOCIAL_ACCOUNT_ID, MAX_LIMIT_EXCEEDED
- 404: NOT_FOUND (read API)
- 409: CONFLICT (duplicate URL)
- 500: INTERNAL_ERROR

12. Sample JSON requests/responses
- Bulk Insert Request:
{
  "source": "social-graph-processor",
  "items": [
    {
      "fk_affiliate_social_account_id": 12345,
      "recommended_social_account_url": "https://www.youtube.com/@example1",
      "recommended_social_account_desc": "Primary YT channel",
      "is_onboarded": 1
    },
    {
      "fk_affiliate_social_account_id": 12345,
      "recommended_social_account_url": "https://instagram.com/example_profile",
      "recommended_social_account_desc": "IG profile",
      "is_onboarded": 0
    }
  ]
}

- Bulk Insert Success Response:
{
  "inserted_ids": [987001, 987002],
  "count": 2
}

- Bulk Insert Failure (Duplicate in payload):
{
  "code": "CONFLICT",
  "message": "duplicate URL for same affiliate social account in request payload",
  "details": {
    "index": 1,
    "url": "https://instagram.com/example_profile",
    "fk_affiliate_social_account_id": 12345
  }
}

- Read API Request:
GET /api/v1/recommended-social-accounts?fk_affiliate_social_account_id=12345,67890

- Read API Success Response:
{
  "data": [
    {
      "fk_affiliate_social_account_id": 12345,
      "channels": [
        {
          "recommended_social_account_id": 987001,
          "recommended_social_account_url": "https://www.youtube.com/@example1",
          "recommended_social_account_desc": "Primary YT channel",
          "is_onboarded": 1,
          "insert_date": "2025-12-03T10:15:30Z",
          "last_update_date": "2025-12-03T10:15:30Z"
        },
        {
          "recommended_social_account_id": 987002,
          "recommended_social_account_url": "https://instagram.com/example_profile",
          "recommended_social_account_desc": "IG profile",
          "is_onboarded": 0,
          "insert_date": "2025-12-03T10:16:10Z",
          "last_update_date": "2025-12-03T10:16:10Z"
        }
      ]
    }
  ],
  "count": 2
}

- Read API Not Found Response:
{
  "code": "NOT_FOUND",
  "message": "no recommended channels found for given affiliate IDs"
}

13. Unit test skeletons
// internal/recommendedsocialaccount/tests/service_test.go
package tests

import (
	"context"
	"testing"
	"time"

	"internal/recommendedsocialaccount/dto"
	"internal/recommendedsocialaccount/service"
)

func TestBulkInsert_Success(t *testing.T) {
	ctx := context.Background()
	req := dto.BulkInsertRequest{
		Source: "unit-test",
		Items: []dto.RecommendedSocialAccountInsertItem{
			{
				FkAffiliateSocialAccountID:   1,
				RecommendedSocialAccountURL:  "https://youtube.com/@unit1",
				RecommendedSocialAccountDesc: "desc1",
				IsOnboarded:                  1,
			},
		},
	}
	// TODO: Mock ExecuteSQL/ExecuteQuery helpers to simulate DB
	_, _ = service.BulkInsertRecommendedSocialAccounts(ctx, req)
	// TODO: Assert response and no error
}

func TestBulkInsert_InvalidURL(t *testing.T) {
	ctx := context.Background()
	req := dto.BulkInsertRequest{
		Source: "unit-test",
		Items: []dto.RecommendedSocialAccountInsertItem{
			{
				FkAffiliateSocialAccountID:   1,
				RecommendedSocialAccountURL:  "ftp://invalid",
				RecommendedSocialAccountDesc: "desc1",
				IsOnboarded:                  1,
			},
		},
	}
	_, apiErr := service.BulkInsertRecommendedSocialAccounts(ctx, req)
	if apiErr == nil || apiErr.Code != "BAD_REQUEST" {
		t.Fatalf("expected BAD_REQUEST for invalid URL, got: %+v", apiErr)
	}
}

func TestBulkInsert_MaxLimitExceeded(t *testing.T) {
	ctx := context.Background()
	var items []dto.RecommendedSocialAccountInsertItem
	for i := 0; i < 26; i++ {
		now := time.Now()
		items = append(items, dto.RecommendedSocialAccountInsertItem{
			FkAffiliateSocialAccountID:   1,
			RecommendedSocialAccountURL:  "https://example.com/" + string(rune('a'+(i%26))),
			RecommendedSocialAccountDesc: "desc",
			IsOnboarded:                  0,
			InsertDate:                   &now,
			LastUpdateDate:               &now,
		})
	}
	req := dto.BulkInsertRequest{Items: items}
	_, apiErr := service.BulkInsertRecommendedSocialAccounts(ctx, req)
	if apiErr == nil || apiErr.Code != "MAX_LIMIT_EXCEEDED" {
		t.Fatalf("expected MAX_LIMIT_EXCEEDED, got: %+v", apiErr)
	}
}

func TestRead_NotFound(t *testing.T) {
	ctx := context.Background()
	_, apiErr := service.ReadRecommendedSocialAccounts(ctx, dto.ReadRequest{FkAffiliateSocialAccountIDs: []int64{999999}})
	// TODO: With mocks: return empty set -> expect NOT_FOUND
	_ = apiErr
}

14. Validator helper (optional lightweight)
// internal/recommendedsocialaccount/validator/validator.go
package validator

// Placeholder for future shared validation logic if needed.

Notes and Assumptions
- Transaction handling: The repository uses ExecuteSQL(ctx, "BEGIN"/"COMMIT"/"ROLLBACK") to enforce atomic inserts, per the provided helpers. This assumes the helpers maintain connection affinity for the context. If the project provides a transactional helper, it can replace this pattern without changing service logic.
- Parameterized queries: All queries use "?" placeholders. If the project uses PostgreSQL with $1-style parameters, adapt placeholderList accordingly.
- Insert IDs: INSERT ... RETURNING is used to fetch inserted IDs in a DB-agnostic way for systems supporting RETURNING (e.g., PostgreSQL). If using MySQL, switch to res.LastInsertId() with ExecuteSQL and capture IDs per row or use SELECT LAST_INSERT_ID() post insert.
- Index recommendation provided in queries.sql for faster reads on fk_affiliate_social_account_id.

Routing
- POST /api/v1/recommended-social-accounts/bulk → controller.BulkInsertRecommendedSocialAccountsHandler
- GET /api/v1/recommended-social-accounts → controller.ReadRecommendedSocialAccountsHandler (query: fk_affiliate_social_account_id=1,2,...)

This completes the required implementation per BRD, with clean layered architecture, validation, logging, error mapping, and sample tests.