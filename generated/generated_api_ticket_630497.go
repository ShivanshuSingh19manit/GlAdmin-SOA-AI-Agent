File: controller/product/product_image_lookup.go
-----------------------------------------------
package controller

import (
	"GLADMIN-RAPI/dbase"
	"GLADMIN-RAPI/logs"
	"GLADMIN-RAPI/utils"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// ProductImageLookup handles product image related read operations using "case"
func ProductImageLookup(w http.ResponseWriter, r *http.Request) {

	start := time.Now()

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		logs.Log.Error("error occurred while reading request body: ", err)
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	var req_data map[string]interface{}
	_ = json.Unmarshal([]byte(body), &req_data)

	requestedBy := req_data["requested_by"]

	// keep input copy
	input := make(map[string]interface{})
	for key, value := range req_data {
		input[key] = value
	}

	msg := "success"

	response := make(map[string]interface{})
	response["Code"] = ""
	response["Url"] = r.RequestURI
	response["Input"] = input
	response["Msg"] = msg

	// cleanup non-processing keys
	delete(req_data, "requested_by")

	status_code := "400"

	// generate unique log id
	logid := uuid.New()
	log_id := logid.String()

	var data []map[string]interface{}

	flag := req_data["case"]
	delete(req_data, "case")

	if num, ok := flag.(string); ok {
		switch num {
		case "1":
			// Case 1: Fetch item details linked to an image URL
			status_code, data, err = getProductImageByUrl(req_data, log_id)
		default:
			msg = "pls enter valid case value"
		}
	} else {
		msg = "either case field is not present or value is not string"
	}

	if err != nil {
		msg = fmt.Sprintf("%v", err)
	}

	response["Code"] = status_code
	response["Msg"] = msg
	response["Data"] = data

	log_response, _ := json.Marshal(response)

	logs.Log.WithFields(logrus.Fields{
		"Code":         status_code,
		"response":     string(log_response),
		"Requested_By": requestedBy,
		"TOTAL_TIME":   time.Since(start).Seconds() * 1000,
		"Size":         float64(len(log_response)) / 1024,
		"LOG_ID":       log_id,
		"TYPE":         "RAPI",
		"API":          "ProductImageLookup",
	}).Info(msg)

	json.NewEncoder(w).Encode(response)
}

// =====================================================
// Case 1 Handler for ProductImageLookup
// =====================================================
// Generic Read API: Fetch item_id/glid/image_id/pc_item_img_status by image_url
func getProductImageByUrl(req_data map[string]interface{}, log_id string) (string, []map[string]interface{}, error) {

	var data []map[string]interface{}

	// 1. Validate mandatory field
	if req_data["image_url"] == nil || req_data["image_url"] == "" {
		return "400", data, errors.New("mandatory field/fields - image_url not present in json body")
	}

	// 2. Validate and extract fields
	fields := ""
	if req_data["fields"] != nil {
		str, ok := utils.ValidateColumnStringForPcItemImages(req_data["fields"])
		if !ok {
			return "400", data, errors.New("mandatory field - fields is invalid")
		}
		fields = str
	} else {
		return "400", data, errors.New("mandatory field - fields doesn't exist")
	}

	delete(req_data, "fields")

	// 3. Extract primary key from request
	imageURL := req_data["image_url"]
	delete(req_data, "image_url")

	// 4. Check for extra/unused fields
	if utils.IsMapContainExtraField(req_data) {
		return "400", data, errors.New("unnecessary field/fields exist")
	}

	// 5. Prepare SQL query
	// Fetch by image_url
	sqlStatement := "SELECT " + fields + " FROM PC_ITEM_IMAGES WHERE image_url = $1"

	// 6. Prepare DB query params
	var params []interface{}
	params = append(params, imageURL)

	// 7. Get database connection (SLAVE DB)
	db, err := dbase.GetDB1(log_id)
	if err != nil {
		return "500", data, errors.New("problem in database connection")
	}

	// 8. Execute query
	data, err = dbase.ExecuteQueryAndGetAllRows(db, sqlStatement, params, log_id)
	if err != nil {
		return "500", data, err
	}

	// 9. Format time columns
	data = utils.FormatTimeInArrayOfMaps(data)

	// 10. Return success response
	return "200", data, nil
}


File: utils/product_image_lookup_checks.go
------------------------------------------
package utils

import "strings"

// ValidateColumnStringForPcItemImages validates comma-separated columns for PC_ITEM_IMAGES
func ValidateColumnStringForPcItemImages(fields interface{}) (string, bool) {

	str, ok := fields.(string)
	if !ok {
		return "", false
	}

	trimmed := strings.TrimSpace(str)

	// wildcard support
	if trimmed == "ALL" {
		return "*", true
	}

	arr := strings.Split(trimmed, ",")

	for _, col := range arr {
		if !is_Pc_Item_Images_Col_Exist(strings.ToLower(strings.TrimSpace(col))) {
			return "", false
		}
	}

	return trimmed, true
}

// is_Pc_Item_Images_Col_Exist checks if a column is allowed for PC_ITEM_IMAGES
func is_Pc_Item_Images_Col_Exist(col string) bool {

	allowedCols := []string{
		// Core identifiers and status
		"item_id",
		"glid",
		"image_id",
		"pc_item_img_status",
		// Common supporting columns
		"image_url",
		"created_on",
		"updated_on",
		"created_by",
		"updated_by",
	}

	for _, valid := range allowedCols {
		if valid == col {
			return true
		}
	}

	return false
}