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

package status

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clientu "sigs.k8s.io/cli-experimental/internal/pkg/client/unstructured"
)

func readyConditionReader(u *unstructured.Unstructured) ([]Condition, error) {
	rv := []Condition{}
	ready := false
	obj := u.UnstructuredContent()

	// ensure that the meta generation is observed
	metaGeneration := clientu.GetIntField(obj, ".metadata.generation", -1)
	observedGeneration := clientu.GetIntField(obj, ".status.observedGeneration", metaGeneration)
	if observedGeneration != metaGeneration {
		reason := "Controller has not observed the latest change. Status generation does not match with metadata"
		rv = append(rv, NewCondition(ConditionReady, reason).False().Get())
		return rv, nil
	}

	// Conditions
	conditions := clientu.GetConditions(obj)
	for _, c := range conditions {
		switch clientu.GetStringField(c, "type", "") {
		case "Ready":
			ready = true
			reason := clientu.GetStringField(c, "reason", "")
			if clientu.GetStringField(c, "status", "") == "False" {
				rv = append(rv, NewCondition(ConditionReady, reason).False().Get())
			} else {
				rv = append(rv, NewCondition(ConditionReady, reason).Get())
			}
		}
	}
	if !ready {
		rv = append(rv, NewCondition(ConditionReady, "No Ready condition found").Get())
	}

	return rv, nil
}

// GetGenericReadyFn - True if we handle it as a known type
func GetGenericReadyFn(u *unstructured.Unstructured) GetConditionsFn {
	return readyConditionReader
}
