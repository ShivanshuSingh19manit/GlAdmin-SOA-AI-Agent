Below is a drop-in scaffold you can paste into your monorepo and then replace the placeholders with your actual module, feature, route, tables, and columns. It strictly follows the GLADMIN-API patterns and constraints you specified.

File: gladmin-api/<MODULE_NAME>/controller/<feature>.go
```go
// Package controller implements <feature> CRUD following GLADMIN-API patterns.
/*
Function dictionary (existing/common helpers):
- dbase.GetDB1(log_id): returns pooled *sql.DB
- dbase.QueryExecuteparam(sql string, db *sql.DB, params []interface{}, log_id string) (*sql.Rows, error)
- dbase.ExecuteWriteQuery(sql string, db *sql.DB, params []interface{}, log_id string) (string, sql.Result, error)
- dbase.DataExecuteParams(sql string, db *sql.DB, params []interface{}, log_id string) (string, int64, error)
- logs.Log.WithFields(fields logrus.Fields): structured JSON logging
- utils.SanitizeMap(map[string]interface{}) (map[string]interface{}, error): cleanses inputs, if available in this repo
*/
package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"gladmin-api/common/dbase"
	"gladmin-api/common/logs"
	"gladmin-api/common/utils"
)

// Replace the placeholders below with your actual configuration.
var (
	// SQL target table(s)
	tableName  = "<primary_table>"      // e.g., "products"
	primaryKey = "<pk column>"          // e.g., "id"

	// Columns
	createColumns      = []string{ /* "<col1>", "<col2>", ... */ }
	updateColumns      = []string{ /* "<updatable_col1>", "<updatable_col2>", ... */ }
	readSelectableCols = []string{ /* "<select_col1>", "<select_col2>", ... */ } // if empty => "*"
	readFilterCols     = []string{ /* "<filter_col1>", "<filter_col2>", ... */ }

	// Read defaults
	defaultOrderBy = "" // e.g., "id DESC"
)

// <HandlerName> handles CRUD operations for <feature> based on the "action" field in the request body.
// Request body pattern: {"action":"read|create|update|delete", ...other_fields}
// It removes audit-only fields before DB use: action, AK, VALIDATION_KEY, updated_by_id.
func <HandlerName>(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	w.Header().Set("Content-Type", "application/json")

	logID := uuid.New().String()

	// Read body
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		resp := map[string]interface{}{
			"Code":   "400",
			"Url":    r.URL.Path,
			"Method": "",
			"Input":  map[string]interface{}{},
			"Msg":    "unable to read request body",
		}
		writeAndLog(w, resp, "400", logID, start, rawBody, nil)
		return
	}
	defer r.Body.Close()

	// Decode into map[string]interface{}
	var req map[string]interface{}
	if err := json.Unmarshal(rawBody, &req); err != nil {
		resp := map[string]interface{}{
			"Code":   "400",
			"Url":    r.URL.Path,
			"Method": "",
			"Input":  string(rawBody),
			"Msg":    "invalid JSON body",
		}
		writeAndLog(w, resp, "400", logID, start, rawBody, err)
		return
	}

	// Keep a copy of raw input for response/logging
	inputCopy := map[string]interface{}{}
	for k, v := range req {
		inputCopy[k] = v
	}

	// Extract action
	actionRaw, ok := req["action"]
	if !ok {
		resp := map[string]interface{}{
			"Code":   "400",
			"Url":    r.URL.Path,
			"Method": "",
			"Input":  inputCopy,
			"Msg":    "request action not found/incorrect",
		}
		writeAndLog(w, resp, "400", logID, start, rawBody, nil)
		return
	}
	action, _ := actionRaw.(string)
	action = strings.ToLower(strings.TrimSpace(action))

	// Remove audit-only/reserved keys before DB use
	reqData := map[string]interface{}{}
	for k, v := range req {
		reqData[k] = v
	}
	delete(reqData, "action")
	delete(reqData, "AK")
	delete(reqData, "VALIDATION_KEY")
	delete(reqData, "updated_by_id")

	// Optional sanitization
	if sanitized, err := utils.SanitizeMap(reqData); err == nil && sanitized != nil {
		reqData = sanitized
	}

	urlPath := r.URL.Path

	switch action {
	case "read":
		code, data, err := read<HandlerName>(req, logID) // pass original req so limit/offset/filters can be read
		resp := map[string]interface{}{
			"Code":   code,
			"Url":    urlPath,
			"Method": action,
			"Input":  inputCopy,
			"Msg":    msgFromErr(err),
			"Data":   data,
		}
		writeAndLog(w, resp, code, logID, start, rawBody, err)
		return

	case "create":
		code, id, err := create<HandlerName>(reqData, logID)
		resp := map[string]interface{}{
			"Code":   code,
			"Url":    urlPath,
			"Method": action,
			"Input":  inputCopy,
			"Msg":    msgFromErr(err),
			"Data":   map[string]interface{}{"id": id},
		}
		writeAndLog(w, resp, code, logID, start, rawBody, err)
		return

	case "update":
		code, affected, err := update<HandlerName>(reqData, logID)
		resp := map[string]interface{}{
			"Code":   code,
			"Url":    urlPath,
			"Method": action,
			"Input":  inputCopy,
			"Msg":    msgFromErr(err),
			"Data":   map[string]interface{}{"rows_affected": affected},
		}
		writeAndLog(w, resp, code, logID, start, rawBody, err)
		return

	case "delete":
		code, affected, err := delete<HandlerName>(reqData, logID)
		resp := map[string]interface{}{
			"Code":   code,
			"Url":    urlPath,
			"Method": action,
			"Input":  inputCopy,
			"Msg":    msgFromErr(err),
			"Data":   map[string]interface{}{"rows_affected": affected},
		}
		writeAndLog(w, resp, code, logID, start, rawBody, err)
		return

	default:
		resp := map[string]interface{}{
			"Code":   "400",
			"Url":    urlPath,
			"Method": action,
			"Input":  inputCopy,
			"Msg":    "request action not found/incorrect",
		}
		writeAndLog(w, resp, "400", logID, start, rawBody, nil)
		return
	}
}

// read<HandlerName> executes a SELECT with optional filters and pagination.
// Expected optional input keys for filters: entries in readFilterCols.
// Optional pagination: limit (int), offset (int).
func read<HandlerName>(data map[string]interface{}, logID string) (string, []map[string]interface{}, error) {
	db, _ := dbase.GetDB1(logID)

	cols := "*"
	if len(readSelectableCols) > 0 {
		cols = strings.Join(readSelectableCols, ", ")
	}

	sqlStr := fmt.Sprintf("SELECT %s FROM %s", cols, tableName)
	var where []string
	var params []interface{}

	// Apply filters present in data and allowed by readFilterCols
	allowed := make(map[string]bool, len(readFilterCols))
	for _, c := range readFilterCols {
		allowed[c] = true
	}
	for k, v := range data {
		if allowed[k] {
			where = append(where, fmt.Sprintf("%s = $%d", k, len(params)+1))
			params = append(params, v)
		}
	}

	if len(where) > 0 {
		sqlStr += " WHERE " + strings.Join(where, " AND ")
	}
	if defaultOrderBy != "" {
		sqlStr += " ORDER BY " + defaultOrderBy
	}

	// Pagination
	limit, hasLimit := intFromAny(data["limit"])
	offset, hasOffset := intFromAny(data["offset"])
	if hasLimit && limit > 0 {
		sqlStr += fmt.Sprintf(" LIMIT $%d", len(params)+1)
		params = append(params, limit)
	}
	if hasOffset && offset >= 0 {
		sqlStr += fmt.Sprintf(" OFFSET $%d", len(params)+1)
		params = append(params, offset)
	}

	rows, err := dbase.QueryExecuteparam(sqlStr, db, params, logID)
	if err != nil {
		return "500", nil, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return "500", nil, err
	}

	var result []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			return "500", nil, err
		}

		rowMap := make(map[string]interface{}, len(columns))
		for i, col := range columns {
			val := values[i]
			switch t := val.(type) {
			case []byte:
				rowMap[col] = string(t)
			default:
				rowMap[col] = t
			}
		}
		result = append(result, rowMap)
	}

	return "200", result, nil
}

// create<HandlerName> inserts a new record and returns the inserted primary key.
func create<HandlerName>(data map[string]interface{}, logID string) (string, int64, error) {
	db, _ := dbase.GetDB1(logID)

	if len(createColumns) == 0 {
		return "400", 0, fmt.Errorf("no create columns configured")
	}

	var params []interface{}
	var placeholders []string
	for i, col := range createColumns {
		params = append(params, data[col])
		placeholders = append(placeholders, fmt.Sprintf("$%d", i+1))
	}

	sqlStr := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) RETURNING %s",
		tableName, strings.Join(createColumns, ", "), strings.Join(placeholders, ", "), primaryKey)

	code, id, err := dbase.DataExecuteParams(sqlStr, db, params, logID)
	return code, id, err
}

// update<HandlerName> updates a record using primary key in data[primaryKey] and returns rows affected.
func update<HandlerName>(data map[string]interface{}, logID string) (string, int64, error) {
	db, _ := dbase.GetDB1(logID)

	pkVal, ok := data[primaryKey]
	if !ok || pkVal == nil {
		return "400", 0, fmt.Errorf("missing primary key: %s", primaryKey)
	}

	var setParts []string
	var params []interface{}
	for _, col := range updateColumns {
		if val, ok := data[col]; ok {
			params = append(params, val)
			setParts = append(setParts, fmt.Sprintf("%s = $%d", col, len(params)))
		}
	}

	if len(setParts) == 0 {
		return "400", 0, fmt.Errorf("no updatable columns provided")
	}

	// WHERE ... pk
	params = append(params, pkVal)
	sqlStr := fmt.Sprintf("UPDATE %s SET %s WHERE %s = $%d",
		tableName, strings.Join(setParts, ", "), primaryKey, len(params))

	code, res, err := dbase.ExecuteWriteQuery(sqlStr, db, params, logID)
	if err != nil {
		return code, 0, err
	}
	affected, _ := res.RowsAffected()
	return code, affected, nil
}

// delete<HandlerName> deletes a record by primary key and returns rows affected.
func delete<HandlerName>(data map[string]interface{}, logID string) (string, int64, error) {
	db, _ := dbase.GetDB1(logID)

	pkVal, ok := data[primaryKey]
	if !ok || pkVal == nil {
		return "400", 0, fmt.Errorf("missing primary key: %s", primaryKey)
	}

	sqlStr := fmt.Sprintf("DELETE FROM %s WHERE %s = $1", tableName, primaryKey)
	params := []interface{}{pkVal}

	code, res, err := dbase.ExecuteWriteQuery(sqlStr, db, params, logID)
	if err != nil {
		return code, 0, err
	}
	affected, _ := res.RowsAffected()
	return code, affected, nil
}

// Utility: write response and Kibana-friendly logs, without panicking.
func writeAndLog(w http.ResponseWriter, response map[string]interface{}, code string, logID string, start time.Time, rawBody []byte, err error) {
	// Attempt to include updated_by_id if provided in input
	var updatedBy interface{}
	if in, ok := response["Input"].(map[string]interface{}); ok {
		if v, exists := in["updated_by_id"]; exists {
			updatedBy = v
		}
	}
	// TOTAL_TIME in ms
	totalMs := time.Since(start).Milliseconds()
	// Size in KB
	sizeKB := float64(len(rawBody)) / 1024.0

	// Stringify response for logs
	respBytes, _ := json.Marshal(response)
	respStr := string(respBytes)

	fields := map[string]interface{}{
		"Code":       code,
		"response":   respStr,
		"TOTAL_TIME": totalMs,
		"Size":       fmt.Sprintf("%.2f", sizeKB),
		"LOG_ID":     logID,
	}
	if updatedBy != nil {
		fields["updated_by_id"] = updatedBy
	}

	if err != nil {
		logs.Log.WithFields(fields).Error(err)
	} else {
		logs.Log.WithFields(fields).Info("OK")
	}

	_ = json.NewEncoder(w).Encode(response)
}

// Utility: best-effort extract int from interface{}, returns (value, ok)
func intFromAny(v interface{}) (int, bool) {
	switch t := v.(type) {
	case int:
		return t, true
	case int32:
		return int(t), true
	case int64:
		return int(t), true
	case float64:
		// JSON numbers decode as float64
		return int(t), true
	case json.Number:
		i, err := t.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	default:
		return 0, false
	}
}

// Utility: simple message from error or "success"
func msgFromErr(err error) string {
	if err != nil {
		return err.Error()
	}
	return "success"
}
```

Route registration edit snippet
Add the following lines to gladmin-api/<MODULE_NAME>/<MODULE_NAME>main.go, ensuring you keep any existing middlewares (like AK.AKValidationMiddleware) intact:

```go
// Route: /<route_name>
r.HandleFunc("/<route_name>", controller.<HandlerName>).Methods("POST")
r.HandleFunc("/<route_name>/", controller.<HandlerName>).Methods("POST")
```

How to fill the placeholders
- <MODULE_NAME>: your module directory, e.g., product, user, marketplace, administration, fcp, buylead.
- <feature>: a short feature/file name for the handler file, e.g., item, profile, category.
- <HandlerName>: a PascalCase function name, e.g., ItemCRUD, ProfileCRUD.
- tableName: primary table name, e.g., "products".
- primaryKey: column name of the PK, e.g., "id".
- createColumns: ordered list of columns to insert, e.g., []string{"name","sku","price","status"}.
- updateColumns: columns allowed to be updated (without the PK), e.g., []string{"name","price","status"}.
- readSelectableCols: columns to select for reads. If empty, "*" is used.
- readFilterCols: whitelisted filter columns, e.g., []string{"id","status","sku"}.
- defaultOrderBy: optional ORDER BY clause, e.g., "id DESC".

Notes
- Request body must include an action key: "read", "create", "update", or "delete".
- The handler strips action, AK, VALIDATION_KEY, and updated_by_id prior to DB use, then optionally sanitizes via utils.SanitizeMap.
- All SQL uses positional parameters $1, $2, ... and the dbase helpers to avoid SQL injection and to follow repo conventions.
- Logging includes Code, response, updated_by_id (if provided), TOTAL_TIME (ms), Size (KB), and LOG_ID, using logs.Log.WithFields and logs.Log.Error for errors.
- Responses are always JSON encoded via json.NewEncoder(w).Encode(response) with the shape: Code, Url, Method, Input, Msg, and Data (for read and write outcomes).