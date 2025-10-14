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
	"reflect"
	"strings"
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
	} else if action == "business_requirement" {
		status_code, numbers, err = processBusinessRequirement(sanitise_data, log_id)
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
	// connect to the databse
	db, err := dbase.GetDB1(log_id)
	if err != nil {
		return "500", []int64{-1}, errors.New("problem in database connection")
	}

	colCount := 1
	var params []interface{}
	sqlStatement := "select glusr_usr_id,glusr_usr_firstname,glusr_usr_lastname,glusr_usr_companyname,glusr_usr_email,glusr_usr_ph_mobile,glusr_usr_ph_country,glusr_usr_mobile_country,glusr_usr_ph_number from glusr_usr"
	sqlStatement1 := "select glusr_usr_id from glusr_usr "
	for key, value := range req_data {

		t := fmt.Sprintf("%T", value)
		if value == nil || (t == "string" && value == "") {
			continue
		}

		if key == "glusr_usr_ph_mobile" {
			sqlStatement += " where glusr_usr_ph_mobile=" + fmt.Sprintf(`$%d`, colCount) + ";"
			sqlStatement1 += " where glusr_usr_ph_mobile=" + fmt.Sprintf(`$%d`, colCount) + ";"
			params = append(params, value)
		} else if key == "glusr_usr_email_dup" {
			sqlStatement += " where glusr_usr_email_dup=UPPER(" + fmt.Sprintf(`$%d`, colCount) + ");"
			sqlStatement1 += " where glusr_usr_email_dup=UPPER(" + fmt.Sprintf(`$%d`, colCount) + ");"
			params = append(params, value)
		} else {
			return "400", []int64{-1}, errors.New("mandatory field/fields doesn't exist in json")
		}

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
	err = rows.Err()
	if err != nil {
		return "400", []int64{-1}, err
	}

	status, err := dbase.ExecuteParam(sqlStatement, db, params, log_id)
	return status, numbers, err
}

func processBusinessRequirement(req_data map[string]interface{}, log_id string) (string, []int64, error) {
	// Expected JSON shape:
	// {
	//   "Business_Requirement": {
	//     "API": "string",
	//     "API_Input": { ... } OR [ ... ],
	//     "API_Output": [ ... ] OR { ... },
	//     "Validation": {
	//        "required_fields": ["field1","field2"],
	//        "field_types": {"field1":"string","field2":"number"}
	//     }
	//   }
	// }
	brRaw, ok := req_data["Business_Requirement"]
	if !ok || brRaw == nil {
		return "400", []int64{-1}, errors.New("missing Business_Requirement in request")
	}
	brMap, ok := brRaw.(map[string]interface{})
	if !ok {
		return "400", []int64{-1}, errors.New("Business_Requirement must be an object")
	}

	// Validate API name
	apiRaw, ok := brMap["API"]
	if !ok || apiRaw == nil {
		return "400", []int64{-1}, errors.New("API not specified in Business_Requirement")
	}
	apiName, ok := apiRaw.(string)
	if !ok || strings.TrimSpace(apiName) == "" {
		return "400", []int64{-1}, errors.New("API must be a non-empty string")
	}

	// Validate API Input
	apiInput, ok := brMap["API_Input"]
	if !ok {
		return "400", []int64{-1}, errors.New("API_Input not specified in Business_Requirement")
	}
	if _, isMap := apiInput.(map[string]interface{}); !isMap {
		if _, isArr := apiInput.([]interface{}); !isArr {
			return "400", []int64{-1}, errors.New("API_Input must be object or array")
		}
	}

	// Validate API Output
	apiOutput, ok := brMap["API_Output"]
	if !ok {
		return "400", []int64{-1}, errors.New("API_Output not specified in Business_Requirement")
	}
	if _, isMap := apiOutput.(map[string]interface{}); !isMap {
		if _, isArr := apiOutput.([]interface{}); !isArr {
			return "400", []int64{-1}, errors.New("API_Output must be object or array")
		}
	}

	// Optional validations
	valRaw, hasValidation := brMap["Validation"]
	if hasValidation && valRaw != nil {
		valMap, ok := valRaw.(map[string]interface{})
		if !ok {
			return "400", []int64{-1}, errors.New("Validation must be an object if provided")
		}
		// required_fields validation (only if API_Input is an object)
		if reqFieldsRaw, ok := valMap["required_fields"]; ok && reqFieldsRaw != nil {
			reqFieldsArr, ok := interfaceSliceToStringSlice(reqFieldsRaw)
			if !ok {
				return "400", []int64{-1}, errors.New("Validation.required_fields must be an array of strings")
			}
			if len(reqFieldsArr) > 0 {
				if inputObj, ok := apiInput.(map[string]interface{}); ok {
					missing := missingRequiredFields(inputObj, reqFieldsArr)
					if len(missing) > 0 {
						return "400", []int64{-1}, fmt.Errorf("missing required input fields: %s", strings.Join(missing, ", "))
					}
				} else {
					return "400", []int64{-1}, errors.New("required_fields validation applicable only when API_Input is an object")
				}
			}
		}
		// field_types validation (only if API_Input is an object)
		if typeRulesRaw, ok := valMap["field_types"]; ok && typeRulesRaw != nil {
			typeRules, ok := typeRulesRaw.(map[string]interface{})
			if !ok {
				return "400", []int64{-1}, errors.New("Validation.field_types must be an object")
			}
			if inputObj, ok := apiInput.(map[string]interface{}); ok {
				if err := validateFieldTypes(inputObj, typeRules); err != nil {
					return "400", []int64{-1}, err
				}
			} else {
				return "400", []int64{-1}, errors.New("field_types validation applicable only when API_Input is an object")
			}
		}
	}

	// If everything validated correctly
	return "200", []int64{-1}, nil
}

func interfaceSliceToStringSlice(v interface{}) ([]string, bool) {
	arr, ok := v.([]interface{})
	if !ok {
		return nil, false
	}
	var out []string
	for _, itm := range arr {
		s, ok := itm.(string)
		if !ok {
			return nil, false
		}
		s = strings.TrimSpace(s)
		if s == "" {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}

func missingRequiredFields(input map[string]interface{}, required []string) []string {
	var missing []string
	for _, f := range required {
		v, ok := input[f]
		if !ok || isEmptyValue(v) {
			missing = append(missing, f)
		}
	}
	return missing
}

func isEmptyValue(v interface{}) bool {
	if v == nil {
		return true
	}
	switch val := v.(type) {
	case string:
		return strings.TrimSpace(val) == ""
	case []interface{}:
		return len(val) == 0
	case map[string]interface{}:
		return len(val) == 0
	default:
		// for numbers and booleans, consider non-empty
		return false
	}
}

func validateFieldTypes(input map[string]interface{}, rules map[string]interface{}) error {
	var errs []string
	for key, expectedRaw := range rules {
		expected, ok := expectedRaw.(string)
		if !ok {
			errs = append(errs, fmt.Sprintf("field_types[%s] must be string", key))
			continue
		}
		expected = strings.ToLower(strings.TrimSpace(expected))
		val, exists := input[key]
		if !exists {
			// missing may be handled by required_fields
			continue
		}
		if !typeMatches(val, expected) {
			errs = append(errs, fmt.Sprintf("field %s must be of type %s", key, expected))
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func typeMatches(v interface{}, expected string) bool {
	switch expected {
	case "string":
		_, ok := v.(string)
		return ok
	case "number", "float", "int", "integer":
		// JSON numbers unmarshal to float64 by default
		switch v.(type) {
		case float64, float32, int, int32, int64, uint, uint32, uint64:
			return true
		default:
			// sometimes numbers may be string encoded; we strictly enforce numeric types
			return false
		}
	case "boolean", "bool":
		_, ok := v.(bool)
		return ok
	case "array", "list":
		_, ok := v.([]interface{})
		return ok
	case "object", "map":
		_, ok := v.(map[string]interface{})
		return ok
	case "null", "nil":
		return v == nil || (reflect.ValueOf(v).Kind() == reflect.Interface && reflect.ValueOf(v).IsNil())
	default:
		// unknown type alias, treat as mismatch
		return false
	}
}