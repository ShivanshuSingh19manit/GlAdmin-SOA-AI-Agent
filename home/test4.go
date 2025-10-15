package controller

import (
	"GLADMIN-API/dbase"
	"GLADMIN-API/logs"
	"GLADMIN-API/utils"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// GetUserDetails handles read/create/update/delete actions for user_master table
func GetUserDetails(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	log_id := uuid.New().String()

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var req_data map[string]interface{}
	json.Unmarshal(body, &req_data)

	action := fmt.Sprintf("%v", req_data["action"])
	AK := fmt.Sprintf("%v", req_data["AK"])
	updated_by_id := fmt.Sprintf("%v", req_data["updated_by_id"])

	sanitised := utils.SanitizeMap(req_data)

	response := map[string]interface{}{
		"Code":   "",
		"Url":    r.RequestURI,
		"Method": action,
		"Input":  req_data,
		"Msg":    "success",
	}

	logs.Log.WithFields(logrus.Fields{
		"LOG_ID":        log_id,
		"ACTION":        action,
		"UPDATED_BY_ID": updated_by_id,
		"AK":            AK,
		"REQUEST":       req_data,
		"API":           "GetUserDetails",
	}).Info("API Request Received")

	var status_code string
	var data interface{}
	var errResp error

	if action == "read" {
		status_code, data, errResp = readGetUserDetails(sanitised, log_id)
	} else if action == "create" {
		status_code, data, errResp = createGetUserDetails(sanitised, log_id)
	} else if action == "update" {
		status_code, data, errResp = updateGetUserDetails(sanitised, log_id)
	} else if action == "delete" {
		status_code, data, errResp = deleteGetUserDetails(sanitised, log_id)
	} else {
		response["Code"] = "400"
		response["Msg"] = "Invalid or missing action"
		json.NewEncoder(w).Encode(response)
		return
	}

	if errResp != nil {
		response["Code"] = status_code
		response["Msg"] = errResp.Error()
	} else {
		response["Code"] = status_code
		response["Msg"] = "success"
		response["Data"] = data
	}

	logs.Log.WithFields(logrus.Fields{
		"LOG_ID":   log_id,
		"API":      "GetUserDetails",
		"ELAPSED":  time.Since(start).String(),
		"RESPONSE": response,
	}).Info("API Response Sent")

	json.NewEncoder(w).Encode(response)
}

// readGetUserDetails fetches matching user IDs from user_master
func readGetUserDetails(req_data map[string]interface{}, log_id string) (string, []int64, error) {
	db, err := dbase.GetDB1(log_id)
	if err != nil {
		return "500", []int64{-1}, errors.New("problem in database connection")
	}

	colCount := 1
	var params []interface{}
	sqlQuery := "SELECT id FROM user_master WHERE 1=1"

	for key, value := range req_data {
		if value == nil || value == "" {
			continue
		}
		// ignore meta keys
		if key == "action" || key == "AK" {
			continue
		}
		// only allow safe/known columns
		if !isAllowedColumn(key) {
			continue
		}
		sqlQuery += fmt.Sprintf(" AND %s=$%d", key, colCount)
		params = append(params, value)
		colCount++
	}

	rows, err := dbase.QueryExecuteparam(sqlQuery, db, params, log_id)
	if err != nil {
		return "400", []int64{-1}, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return "400", []int64{-1}, err
		}
		ids = append(ids, id)
	}

	return "200", ids, nil
}

// createGetUserDetails inserts a new user into user_master and returns the created id
func createGetUserDetails(req_data map[string]interface{}, log_id string) (string, map[string]interface{}, error) {
	db, err := dbase.GetDB1(log_id)
	if err != nil {
		return "500", nil, errors.New("problem in database connection")
	}

	var cols []string
	var placeholders []string
	var params []interface{}
	colCount := 1

	// Prepare insert columns from request, skipping meta keys
	for key, value := range req_data {
		if value == nil || key == "" {
			continue
		}
		if key == "action" || key == "AK" || key == "id" {
			continue
		}
		if !isAllowedColumn(key) {
			continue
		}
		cols = append(cols, key)
		placeholders = append(placeholders, fmt.Sprintf("$%d", colCount))
		params = append(params, value)
		colCount++
	}

	if len(cols) == 0 {
		return "400", nil, errors.New("no valid fields to insert")
	}

	// Build column and placeholder lists manually
	colList := ""
	valList := ""
	for i := 0; i < len(cols); i++ {
		if i == 0 {
			colList = cols[i]
			valList = placeholders[i]
		} else {
			colList += ", " + cols[i]
			valList += ", " + placeholders[i]
		}
	}

	// Add audit timestamps
	sqlQuery := fmt.Sprintf(
		"INSERT INTO user_master (%s, created_at, updated_at) VALUES (%s, NOW(), NOW()) RETURNING id",
		colList, valList,
	)

	rows, err := dbase.QueryExecuteparam(sqlQuery, db, params, log_id)
	if err != nil {
		return "400", nil, err
	}
	defer rows.Close()

	var id int64
	if rows.Next() {
		if err := rows.Scan(&id); err != nil {
			return "400", nil, err
		}
	} else {
		return "400", nil, errors.New("insert failed")
	}

	return "200", map[string]interface{}{"created_id": id}, nil
}

// updateGetUserDetails updates an existing user_master row using id and returns the updated id
func updateGetUserDetails(req_data map[string]interface{}, log_id string) (string, map[string]interface{}, error) {
	db, err := dbase.GetDB1(log_id)
	if err != nil {
		return "500", nil, errors.New("problem in database connection")
	}

	id, ok := req_data["id"]
	if !ok || id == nil || fmt.Sprintf("%v", id) == "" {
		return "400", nil, errors.New("missing id for update")
	}

	var setClauses []string
	var params []interface{}
	colCount := 1

	for key, value := range req_data {
		if value == nil || key == "" {
			continue
		}
		if key == "action" || key == "AK" || key == "id" {
			continue
		}
		if !isAllowedColumn(key) {
			continue
		}
		setClauses = append(setClauses, fmt.Sprintf("%s=$%d", key, colCount))
		params = append(params, value)
		colCount++
	}

	if len(setClauses) == 0 {
		return "400", nil, errors.New("no valid fields to update")
	}

	// Build SET clause
	setList := ""
	for i := 0; i < len(setClauses); i++ {
		if i == 0 {
			setList = setClauses[i]
		} else {
			setList += ", " + setClauses[i]
		}
	}
	// add updated_at
	setList += ", updated_at=NOW()"

	// WHERE id
	params = append(params, id)
	sqlQuery := fmt.Sprintf("UPDATE user_master SET %s WHERE id=$%d RETURNING id", setList, colCount)

	rows, err := dbase.QueryExecuteparam(sqlQuery, db, params, log_id)
	if err != nil {
		return "400", nil, err
	}
	defer rows.Close()

	var updatedID int64
	if rows.Next() {
		if err := rows.Scan(&updatedID); err != nil {
			return "400", nil, err
		}
	} else {
		return "400", nil, errors.New("update failed or no rows affected")
	}

	return "200", map[string]interface{}{"updated_id": updatedID}, nil
}

// deleteGetUserDetails deletes user_master rows using id or filters and returns deleted ids
func deleteGetUserDetails(req_data map[string]interface{}, log_id string) (string, map[string]interface{}, error) {
	db, err := dbase.GetDB1(log_id)
	if err != nil {
		return "500", nil, errors.New("problem in database connection")
	}

	var sqlQuery string
	var params []interface{}
	colCount := 1

	if id, ok := req_data["id"]; ok && id != nil && fmt.Sprintf("%v", id) != "" {
		sqlQuery = "DELETE FROM user_master WHERE id=$1 RETURNING id"
		params = append(params, id)
	} else {
		sqlQuery = "DELETE FROM user_master WHERE 1=1"
		for key, value := range req_data {
			if value == nil || value == "" {
				continue
			}
			if key == "action" || key == "AK" {
				continue
			}
			if !isAllowedColumn(key) {
				continue
			}
			sqlQuery += fmt.Sprintf(" AND %s=$%d", key, colCount)
			params = append(params, value)
			colCount++
		}
		if colCount == 1 {
			return "400", nil, errors.New("refusing to delete without filters")
		}
	}
	rows, err := dbase.QueryExecuteparam(sqlQuery, db, params, log_id)
	if err != nil {
		return "400", nil, err
	}
	defer rows.Close()

	var deletedIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return "400", nil, err
		}
		deletedIDs = append(deletedIDs, id)
	}

	return "200", map[string]interface{}{"deleted_ids": deletedIDs}, nil
}

// isAllowedColumn restricts columns used in dynamic SQL to prevent injection via keys
func isAllowedColumn(col string) bool {
	allowed := map[string]bool{
		"id":            true,
		"user_id":       true,
		"email":         true,
		"phone":         true,
		"status":        true,
		"name":          true,
		"role":          true,
		"updated_by_id": true,
		"created_by_id": true,
	}
	return allowed[col]
}