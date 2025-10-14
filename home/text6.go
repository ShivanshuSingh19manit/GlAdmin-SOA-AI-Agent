package controller

import (
	"GLADMIN-API/dbase"
	"GLADMIN-API/logs"
	"GLADMIN-API/utils"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

func Glusrusr(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		logs.Log.Error("error occured: ", err)
		log.Fatal(err)
	}

	var req_data map[string]interface{}
	json.Unmarshal([]byte(body), &req_data)

	action := req_data["action"]
	updated_by_id := req_data["updated_by_id"]

	input := make(map[string]interface{})
	for key, value := range req_data {
		input[key] = value
	}
	msg := "success"
	numbers := []int64{-1}
	response := make(map[string]interface{})
	response["Code"] = ""
	response["Url"] = r.RequestURI
	response["Method"] = action
	response["Input"] = input
	response["Msg"] = msg

	delete(req_data, "action")
	delete(req_data, "AK")
	delete(req_data, "VALIDATION_KEY")
	delete(req_data, "updated_by_id")

	status_code := "400"

	i_d := uuid.New()
	log_id := i_d.String()
	sanitise_data, _ := utils.SanitizeMap(req_data)

	if action == "read" {
		status_code, numbers, err = readglusrusr(sanitise_data, log_id)
	} else {
		msg = "request action not found/incorrect"
	}

	if err != nil {
		msg = fmt.Sprintf("%v", err)
	}

	response["Code"] = status_code
	response["Msg"] = msg
	response["Glusr_usr_id"] = numbers

	log_response, err := json.Marshal(response)
	if err != nil {
		logs.Log.Error(err)
	}
	logs.Log.WithFields(logrus.Fields{
		"Code":          status_code,
		"response":      string(log_response),
		"updated_by_id": updated_by_id,
		"TOTAL_TIME":    time.Since(start).Seconds() * 1000,
		"Size":          float64(len(log_response)) / 1024,
		"LOG_ID":        log_id,
	}).Info(msg)

	json.NewEncoder(w).Encode(response)
}

func readglusrusr(req_data map[string]interface{}, log_id string) (string, []int64, error) {
	// connect to the database
	db, err := dbase.GetDB1(log_id)
	if err != nil {
		return "500", []int64{-1}, errors.New("problem in database connection")
	}

	// Allowed filters
	allowed := map[string]bool{
		"glusr_usr_ph_mobile": true,
		"glusr_usr_email_dup": true,
	}

	// Validation: ensure exactly one of the allowed filters is provided (non-empty), and no unexpected non-empty keys
	var providedKey string
	var providedVal interface{}

	for key, value := range req_data {
		// skip empty values
		t := fmt.Sprintf("%T", value)
		if value == nil || (t == "string" && value == "") {
			continue
		}

		if !allowed[key] {
			return "400", []int64{-1}, errors.New("mandatory field/fields doesn't exist in json")
		}

		if providedKey != "" {
			return "400", []int64{-1}, errors.New("provide exactly one of glusr_usr_ph_mobile or glusr_usr_email_dup")
		}
		providedKey = key
		providedVal = value
	}

	if providedKey == "" {
		return "400", []int64{-1}, errors.New("mandatory field missing: provide glusr_usr_ph_mobile or glusr_usr_email_dup")
	}

	// Build SQL based on provided filter
	sqlStatement := "select glusr_usr_id,glusr_usr_firstname,glusr_usr_lastname,glusr_usr_companyname,glusr_usr_email,glusr_usr_ph_mobile,glusr_usr_ph_country,glusr_usr_mobile_country,glusr_usr_ph_number from glusr_usr where "
	sqlStatement1 := "select glusr_usr_id from glusr_usr where "
	var params []interface{}

	switch providedKey {
	case "glusr_usr_ph_mobile":
		sqlStatement += "glusr_usr_ph_mobile = $1"
		sqlStatement1 += "glusr_usr_ph_mobile = $1"
		params = append(params, providedVal)
	case "glusr_usr_email_dup":
		sqlStatement += "glusr_usr_email_dup = UPPER($1)"
		sqlStatement1 += "glusr_usr_email_dup = UPPER($1)"
		params = append(params, providedVal)
	default:
		return "400", []int64{-1}, errors.New("mandatory field/fields doesn't exist in json")
	}

	var numbers []int64
	rows, err := dbase.QueryExecuteparam(sqlStatement1, db, params, log_id)
	if err != nil {
		return "400", []int64{-1}, err
	}
	defer rows.Close()

	for rows.Next() {
		var usr_id int64
		err := rows.Scan(&usr_id)
		numbers = append(numbers, usr_id)
		if err != nil {
			return "400", []int64{-1}, err
		}
	}

	if err = rows.Err(); err != nil {
		return "400", []int64{-1}, err
	}

	status, err := dbase.ExecuteParam(sqlStatement, db, params, log_id)
	return status, numbers, err
}