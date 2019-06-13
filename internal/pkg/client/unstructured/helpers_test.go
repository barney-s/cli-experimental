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

package unstructured_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	helperu "sigs.k8s.io/cli-experimental/internal/pkg/client/unstructured"
)

var emptyObj = map[string]interface{}{}
var testObj = map[string]interface{}{
	"f1": map[string]interface{}{
		"f2": map[string]interface{}{
			"i32":   int32(32),
			"i64":   int64(64),
			"float": 64.02,
			"ms": []interface{}{
				map[string]interface{}{"f1f2ms0f1": 22},
				map[string]interface{}{"f1f2ms1f1": "index1"},
			},
			"msbad": []interface{}{
				map[string]interface{}{"f1f2ms0f1": 22},
				32,
			},
		},
	},

	"ride": "dragon",

	"status": map[string]interface{}{
		"conditions": []interface{}{
			map[string]interface{}{"f1f2ms0f1": 22},
			map[string]interface{}{"f1f2ms1f1": "index1"},
		},
	},
}

func TestGetIntField(t *testing.T) {
	v := helperu.GetIntField(testObj, ".f1.f2.i32", -1)
	assert.Equal(t, int(32), v)

	v = helperu.GetIntField(testObj, ".f1.f2.wrongname", -1)
	assert.Equal(t, int(-1), v)

	v = helperu.GetIntField(testObj, ".f1.f2.i64", -1)
	assert.Equal(t, int(64), v)

	v = helperu.GetIntField(testObj, ".f1.f2.float", -1)
	assert.Equal(t, int(-1), v)
}

func TestGetStringField(t *testing.T) {
	v := helperu.GetStringField(testObj, ".ride", "horse")
	assert.Equal(t, v, "dragon")

	v = helperu.GetStringField(testObj, ".destination", "north")
	assert.Equal(t, v, "north")
}

func TestNestedMapSlice(t *testing.T) {
	v, found, err := helperu.NestedMapSlice(testObj, "f1", "f2", "ms")
	assert.NoError(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, []map[string]interface{}{
		map[string]interface{}{"f1f2ms0f1": 22},
		map[string]interface{}{"f1f2ms1f1": "index1"},
	}, v)

	v, found, err = helperu.NestedMapSlice(testObj, "f1", "f2", "msbad")
	assert.Error(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, []map[string]interface{}(nil), v)

	v, found, err = helperu.NestedMapSlice(testObj, "f1", "f2", "wrongname")
	assert.NoError(t, err)
	assert.Equal(t, found, false)
	assert.Equal(t, []map[string]interface{}(nil), v)

	v, found, err = helperu.NestedMapSlice(testObj, "f1", "f2", "i64")
	assert.Error(t, err)
	assert.Equal(t, found, true)
	assert.Equal(t, []map[string]interface{}(nil), v)
}

func TestGetConditions(t *testing.T) {
	v := helperu.GetConditions(emptyObj)
	assert.Equal(t, []map[string]interface{}{}, v)

	v = helperu.GetConditions(testObj)
	assert.Equal(t, []map[string]interface{}{
		map[string]interface{}{"f1f2ms0f1": 22},
		map[string]interface{}{"f1f2ms1f1": "index1"},
	}, v)
}
