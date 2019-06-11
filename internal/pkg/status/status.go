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
	"context"
	"fmt"
	"io"

	"gopkg.in/src-d/go-git.v4/plumbing/object"
	//metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/cli-experimental/internal/pkg/client"
	"sigs.k8s.io/cli-experimental/internal/pkg/clik8s"
)

// Condition types
const (
	// Level condition
	ConditionReady   ConditionType = "Ready"
	ConditionSettled ConditionType = "Settled"

	// Terminal condition
	ConditionFailed    ConditionType = "Failed"
	ConditionCompleted ConditionType = "Completed"
)

// Status returns the status for rollouts
type Status struct {
	// DynamicClient is the client used to talk
	// with the cluster
	DynamicClient client.Client
	// Out stores the output
	Out io.Writer
	// Resources is a list of resource configurations
	Resources clik8s.ResourceConfigs
	// Commit is a git commit object
	Commit *object.Commit
}

// ConditionType - condition types
type ConditionType string

// Condition - generic condition
type Condition struct {
	Type   ConditionType
	Status string // metav1.ConditionStatus
	Reason string
}

// ResourceStatus - resource status
type ResourceStatus struct {
	Resource   *unstructured.Unstructured
	Conditions []Condition
	Error      error
}

// Result contains the Status Result
type Result struct {
	Resources []ResourceStatus
}

// NewFalseCondition condition
func NewFalseCondition(ctype ConditionType) Condition {
	return Condition{Type: ctype, Status: "False", Reason: ""}
}

// NewCondition condition
func NewCondition(ctype ConditionType, reason string) *Condition {
	return &Condition{Type: ctype, Status: "True", Reason: reason}
}

// Get - simple wrapper to avoid *
func (c *Condition) Get() Condition {
	return *c
}

// False - set condition to false
func (c *Condition) False() *Condition {
	c.Status = "False"
	return c
}

// GetCondition - condition
func GetCondition(cs []Condition, ct ConditionType) *Condition {
	for i := range cs {
		if cs[i].Type == ct {
			return &cs[i]
		}
	}
	return nil
}

// Do executes the status
func (a *Status) Do() (Result, error) {
	var errs []error
	var rs = []ResourceStatus{}

	fmt.Fprintf(a.Out, "Doing `cli-experimental apply status`\n")
	ctx := context.Background()
	for _, u := range a.Resources {
		err := a.DynamicClient.Get(ctx,
			types.NamespacedName{Namespace: u.GetNamespace(), Name: u.GetName()}, u)
		if err != nil {
			rs = append(rs, ResourceStatus{Resource: u, Error: err})
			errs = append(errs, err)
			continue
		}

		// Ready indicator is a simple ANDing of all the individual resource readiness
		conditions, err := IsReady(u)
		if err != nil {
			rs = append(rs, ResourceStatus{Resource: u, Error: err})
			errs = append(errs, err)
			continue
		}
		rs = append(rs, ResourceStatus{Resource: u, Conditions: conditions, Error: nil})
	}

	if len(errs) != 0 {
		return Result{Resources: rs}, errors.NewAggregate(errs)
	}
	return Result{Resources: rs}, nil
}

// IsReady - return true if object is ready
func IsReady(u *unstructured.Unstructured) ([]Condition, error) {
	var conditions []Condition
	var err error

	fn := GetLegacyReadyFn(u)
	if fn == nil {
		fn = GetGenericReadyFn(u)
	}

	if fn != nil {
		conditions, err = fn(u)
	}

	return conditions, err
}
