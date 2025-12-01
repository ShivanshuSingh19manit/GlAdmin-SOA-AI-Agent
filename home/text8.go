I’m ready to generate the exact code, but I’m missing the concrete feature details. Please provide:
- Module name (e.g., product, user, marketplace, administration, fcp, buylead)
- Route path (e.g., /product-attributes)
- Handler name (PascalCase)
- Table name(s), primary key column
- Column lists:
  - Create columns
  - Update columns (and where clause, typically by PK)
  - Read filters (including pagination fields if any)
  - Delete rule (by PK or filters)

In the meantime, here is a ready-to-drop template that follows your monorepo’s conventions and can be renamed and completed by replacing the REPLACE_ME values.

File: gladmin-api/<MODULE_NAME>/controller/<feature>.go
package controller

// Function dictionary (project helpers used):
// - dbase.GetDB1(log_id): returns pooled *sql.DB
// - dbase.QueryExecuteparam(query string, db *sql.DB, params []interface{}, log_id string) (*sql.Rows, error)
// - dbase.ExecuteWriteQuery(query string, db *sql.DB, params []interface{}, log_id string) (string, sql.Result, error)
// - dbase.DataExecuteParams(query string, db *sql.DB, params []interface{}, log_id string) (string, int64, error)
// - logs.Log.WithFields(fields).Info(msg)/Error(msg): structured JSON logging (Kibana-friendly)
// - utils.SanitizeMap(map[string]interface{}) (map[string]interface{}, error): Optional, if present in repo

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	// Adjust these imports to your repo layout if different
	"gladmin-api/common/dbase"
	"gladmin-api/common/logs"
	// If utils.SanitizeMap is available, uncomment the next line and use it in the handler where indicated:
	// "gladmin-api/common/utils"
)

// ====== REPLACE_ME: Feature configuration ======
// Replace these values to fit your feature/table
const (
	featureTable = "REPLACE_ME_table" // e.g., "products"
	pkColumn     = "REPLACE_ME_id"    // e.g., "product_id"
)

// Select columns returned by read API (use specific list for predictability)
var selectCols = []string{
	// e.g., "product_id", "name", "sku", "status", "created_at", "updated_at"
	"REPLACE_ME_select_col1",
}

// Columns that can be provided on create
var createCols = []string{
	// e.g., "name", "sku", "status"
	"REPLACE_ME_create_col1",
}

// Columns that can be updated
var updateCols = []string{
	// e.g., "name", "sku", "status"
	"REPLACE_ME_update_col1",
}

// Columns that can be used as filters in read (AND-equality). Add pagination fields separately.
var readFilterable = []string{
	// e.g., "status", "sku"
	"REPLACE_ME_filter_col1",
}

// ====== End of configuration ======

// <HandlerName> handles CRUD for REPLACE_ME feature via { "action": "read|create|update|delete", ... } body
// Keep compatible with AK.AKValidationMiddleware by parsing the JSON body in a standard way.
func FeatureHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Always set JSON Content-Type
	w.Header().Set("Content-Type", "application/json")

	// Read body
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		resp := map[string]interface{}{
			"Code":   "400",
			"Url":    r.URL.Path,
			"Method": "",
			"Input":  nil,
			"Msg":    "failed to read request body",
		}
		_ = json.NewEncoder(w).Encode(resp)
		logWithKibanaFields("400", resp, "", start, len(bodyBytes), "")
		return
	}
	defer r.Body.Close()

	// Unmarshal to map
	var req map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		resp := map[string]interface{}{
			"Code":   "400",
			"Url":    r.URL.Path,
			"Method": "",
			"Input":  string(bodyBytes),
			"Msg":    "invalid JSON body",
		}
		_ = json.NewEncoder(w).Encode(resp)
		logWithKibanaFields("400", resp, "", start, len(bodyBytes), "")
		return
	}

	// Keep a raw copy of input for response/logging
	inputCopy := deepCopyMap(req)

	// Extract action
	action, _ := req["action"].(string)
	action = strings.ToLower(strings.TrimSpace(action))

	// Extract updated_by_id if present for Kibana logging
	updatedBy := ""
	if v, ok := req["updated_by_id"]; ok {
		updatedBy = anyToString(v)
	}

	// Remove reserved/audit keys before passing to DB functions
	delete(req, "action")
	delete(req, "AK")
	delete(req, "VALIDATION_KEY")
	delete(req, "updated_by_id")

	// Optional sanitize (uncomment if utils is available)
	// sanitized, _ := utils.SanitizeMap(req)
	// if sanitized != nil { req = sanitized }

	logID := uuid.New().String()

	var code string
	var msg string
	var respData interface{}

	switch action {
	case "read":
		code, data, err := readFeature(req, logID)
		respData = data
		if err != nil {
			msg = err.Error()
		} else {
			msg = "success"
		}
		code = fallbackCode(code, err)

	case "create":
		code, insertedID, err := createFeature(req, logID)
		respData = map[string]interface{}{"id": insertedID}
		if err != nil {
			msg = err.Error()
		} else {
			msg = "created"
		}
		code = fallbackCode(code, err)

	case "update":
		code, affected, err := updateFeature(req, logID)
		respData = map[string]interface{}{"rows_affected": affected}
		if err != nil {
			msg = err.Error()
		} else {
			msg = "updated"
		}
		code = fallbackCode(code, err)

	case "delete":
		code, affected, err := deleteFeature(req, logID)
		respData = map[string]interface{}{"rows_affected": affected}
		if err != nil {
			msg = err.Error()
		} else {
			msg = "deleted"
		}
		code = fallbackCode(code, err)

	default:
		code = "400"
		msg = "request action not found/incorrect"
		respData = nil
	}

	response := map[string]interface{}{
		"Code":   code,
		"Url":    r.URL.Path,
		"Method": action,
		"Input":  inputCopy,
		"Msg":    msg,
	}
	// Include Data for read; include for other methods if desired
	if action == "read" && respData != nil {
		response["Data"] = respData
	} else if action != "read" && respData != nil {
		response["Data"] = respData
	}

	_ = json.NewEncoder(w).Encode(response)
	logWithKibanaFields(code, response, updatedBy, start, len(bodyBytes), logID)
}

// readFeature executes a parameterized SELECT with AND filters and optional pagination.
func readFeature(data map[string]interface{}, logID string) (string, []map[string]interface{}, error) {
	db, _ := dbase.GetDB1(logID)

	// Build WHERE from allowed filters
	whereParts := []string{}
	params := []interface{}{}
	pIndex := 1

	for _, col := range readFilterable {
		if v, ok := data[col]; ok {
			whereParts = append(whereParts, col+" = $"+intToStr(pIndex))
			params = append(params, v)
			pIndex++
		}
	}

	// Pagination: limit, offset if present as numbers
	limitStr := ""
	offsetStr := ""
	if v, ok := data["limit"]; ok {
		limitStr = " LIMIT $" + intToStr(pIndex)
		params = append(params, v)
		pIndex++
	}
	if v, ok := data["offset"]; ok {
		offsetStr = " OFFSET $" + intToStr(pIndex)
		params = append(params, v)
		pIndex++
	}

	selectList := strings.Join(selectCols, ", ")
	query := "SELECT " + selectList + " FROM " + featureTable
	if len(whereParts) > 0 {
		query += " WHERE " + strings.Join(whereParts, " AND ")
	}
	query += limitStr + offsetStr

	rows, err := dbase.QueryExecuteparam(query, db, params, logID)
	if err != nil {
		return "500", nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return "500", nil, err
	}

	var result []map[string]interface{}
	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return "500", nil, err
		}
		rowMap := make(map[string]interface{}, len(cols))
		for i, c := range cols {
			rowMap[c] = normalizeSQLValue(vals[i])
		}
		result = append(result, rowMap)
	}
	if err := rows.Err(); err != nil {
		return "500", nil, err
	}

	return "200", result, nil
}

// createFeature inserts a row with provided createCols and returns the inserted PK.
func createFeature(data map[string]interface{}, logID string) (string, int64, error) {
	db, _ := dbase.GetDB1(logID)

	cols := []string{}
	holders := []string{}
	params := []interface{}{}
	pIndex := 1

	for _, col := range createCols {
		if v, ok := data[col]; ok {
			cols = append(cols, col)
			holders = append(holders, "$"+intToStr(pIndex))
			params = append(params, v)
			pIndex++
		}
	}

	if len(cols) == 0 {
		return "400", 0, errWrap("no valid columns provided for create")
	}

	query := "INSERT INTO " + featureTable + " (" + strings.Join(cols, ", ") + ") VALUES (" + strings.Join(holders, ", ") + ") RETURNING " + pkColumn
	code, id, err := dbase.DataExecuteParams(query, db, params, logID)
	return code, id, err
}

// updateFeature updates specified columns for a row identified by PK.
func updateFeature(data map[string]interface{}, logID string) (string, int64, error) {
	db, _ := dbase.GetDB1(logID)

	pkVal, ok := data[pkColumn]
	if !ok {
		return "400", 0, errWrap("missing primary key for update")
	}

	sets := []string{}
	params := []interface{}{}
	pIndex := 1
	for _, col := range updateCols {
		if v, ok := data[col]; ok {
			sets = append(sets, col+" = $"+intToStr(pIndex))
			params = append(params, v)
			pIndex++
		}
	}
	if len(sets) == 0 {
		return "400", 0, errWrap("no valid columns provided for update")
	}

	// WHERE pk = $n
	params = append(params, pkVal)
	wherePlaceholder := "$" + intToStr(pIndex)

	query := "UPDATE " + featureTable + " SET " + strings.Join(sets, ", ") + " WHERE " + pkColumn + " = " + wherePlaceholder
	code, res, err := dbase.ExecuteWriteQuery(query, db, params, logID)
	if err != nil {
		return code, 0, err
	}
	affected, _ := res.RowsAffected()
	return code, affected, nil
}

// deleteFeature deletes a row by PK.
func deleteFeature(data map[string]interface{}, logID string) (string, int64, error) {
	db, _ := dbase.GetDB1(logID)

	pkVal, ok := data[pkColumn]
	if !ok {
		return "400", 0, errWrap("missing primary key for delete")
	}

	params := []interface{}{pkVal}
	query := "DELETE FROM " + featureTable + " WHERE " + pkColumn + " = $1"
	code, res, err := dbase.ExecuteWriteQuery(query, db, params, logID)
	if err != nil {
		return code, 0, err
	}
	affected, _ := res.RowsAffected()
	return code, affected, nil
}

// ====== Helpers ======

func logWithKibanaFields(code string, response map[string]interface{}, updatedBy string, start time.Time, bodySizeBytes int, logID string) {
	respJSON, _ := json.Marshal(response)
	totalMs := time.Since(start).Milliseconds()
	sizeKB := float64(bodySizeBytes) / 1024.0

	fields := map[string]interface{}{
		"Code":       code,
		"response":   string(respJSON),
		"TOTAL_TIME": totalMs,
		"Size":       sizeKB,
		"LOG_ID":     logID,
	}
	if updatedBy != "" {
		fields["updated_by_id"] = updatedBy
	}

	// Error level for 4xx/5xx, Info otherwise
	if strings.HasPrefix(code, "4") || strings.HasPrefix(code, "5") {
		logs.Log.WithFields(fields).Error("request completed")
	} else {
		logs.Log.WithFields(fields).Info("request completed")
	}
}

func deepCopyMap(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return nil
	}
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func anyToString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		// JSON numbers become float64
		return strings.TrimRight(strings.TrimRight(fmtFloat(t), "0"), ".")
	case int, int64, int32, uint64, uint:
		return fmtAny(t)
	default:
		b, _ := json.Marshal(t)
		return string(b)
	}
}

func intToStr(i int) string {
	// minimal helper to avoid strconv import if not needed elsewhere
	if i == 0 {
		return "0"
	}
	digits := "0123456789"
	out := []byte{}
	for i > 0 {
		out = append([]byte{digits[i%10]}, out...)
		i /= 10
	}
	return string(out)
}

func normalizeSQLValue(v interface{}) interface{} {
	// Pass through as-is; database/sql already maps driver types.
	// Optionally add special cases if needed for your drivers (e.g., []byte -> string)
	switch t := v.(type) {
	case []byte:
		return string(t)
	default:
		return t
	}
}

func fallbackCode(code string, err error) string {
	if code == "" {
		if err != nil {
			return "500"
		}
		return "200"
	}
	return code
}

// Minimal formatting helpers (no fmt import)
func fmtAny(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
func fmtFloat(f float64) string {
	// return JSON-like compact string for float without importing fmt/strconv
	b, _ := json.Marshal(f)
	return string(b)
}

Route registration snippet (add to gladmin-api/<MODULE_NAME>/<module>main.go):
// Route: REPLACE_ME route name
r.HandleFunc("/REPLACE_ME_route", controller.FeatureHandler).Methods("POST")
r.HandleFunc("/REPLACE_ME_route/", controller.FeatureHandler).Methods("POST")
// If the module uses AK middleware, keep it as-is around the router or this specific route.

Notes:
- Replace all REPLACE_ME placeholders with actual table/column/route details.
- Rename FeatureHandler to your <HandlerName> and rename file accordingly.
- If utils.SanitizeMap is available in your repo, import it and uncomment the sanitize call.
- This handler uses dbase.GetDB1 pooling and only parameterized queries via the *Param helpers.
- The handler logs Kibana-friendly fields including LOG_ID, TOTAL_TIME (ms), Size (KB), and updated_by_id if present.

Once you provide:
- Module, route, handler name
- Table name, PK column
- Create columns, Update columns, Read filters, Delete rule

…I’ll generate the final, concrete file and route lines without placeholders.