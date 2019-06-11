/*
Copyright 2019 The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package unstructured

import (
	"fmt"
	api_unstructured "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"strings"
)

func jsonPath(fields []string) string {
	return "." + strings.Join(fields, ".")
}

// NestedMapSlice returns the value of a nested field.
// Returns false if value is not found and an error if not an slice of maps.
func NestedMapSlice(obj map[string]interface{}, fields ...string) ([]map[string]interface{}, bool, error) {
	val, found, err := api_unstructured.NestedFieldNoCopy(obj, fields...)
	if !found || err != nil {
		return nil, found, err
	}
	array, ok := val.([]interface{})
	if !ok {
		return nil, true, fmt.Errorf("%v accessor error: %v is of the type %T, expected []interface{}", jsonPath(fields), val, val)
	}

	conditions := []map[string]interface{}{}

	for i := range array {
		entry, ok := array[i].(map[string]interface{})
		if !ok {
			return nil, true, fmt.Errorf("%v accessor error: %v[%d] is of the type %T, expected map[string]interface{}", jsonPath(fields), i, val, val)
		}
		conditions = append(conditions, entry)

	}
	return conditions, true, nil
}

// GetStringField - return field as string defaulting to value if not found
func GetStringField(obj map[string]interface{}, fieldPath string, defaultValue string) string {
	var rv = defaultValue

	fields := strings.Split(fieldPath, ".")
	if fields[0] == "" {
		fields = fields[1:]
	}

	val, found, err := api_unstructured.NestedFieldNoCopy(obj, fields...)
	if !found || err != nil {
		return rv
	}

	switch val.(type) {
	case string:
		rv = val.(string)
	}
	return rv
}

// GetIntField - return field as string defaulting to value if not found
func GetIntField(obj map[string]interface{}, fieldPath string, defaultValue int) int {
	var rv = defaultValue

	fields := strings.Split(fieldPath, ".")
	if fields[0] == "" {
		fields = fields[1:]
	}

	val, found, err := api_unstructured.NestedFieldNoCopy(obj, fields...)
	if !found || err != nil {
		return rv
	}

	switch val.(type) {
	case int:
		rv = val.(int)
	case int32:
		rv = int(val.(int32))
	case int64:
		rv = int(val.(int64))
	}
	return rv
}

// GetConditions - return conditions array as []map[string]interface{}
func GetConditions(obj map[string]interface{}) []map[string]interface{} {
	conditions, ok, err := NestedMapSlice(obj, "status", "conditions")
	if err != nil {
		fmt.Printf("err: %s", err)
		return []map[string]interface{}{}
	}
	if !ok {
		return []map[string]interface{}{}
	}
	return conditions
}
