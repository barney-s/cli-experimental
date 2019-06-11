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
	"fmt"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clientu "sigs.k8s.io/cli-experimental/internal/pkg/client/unstructured"
)

// GetConditionsFn - status getter
type GetConditionsFn func(*unstructured.Unstructured) ([]Condition, error)

var legacyTypes = map[string]map[string]GetConditionsFn{
	"": map[string]GetConditionsFn{
		"Service":               serviceConditions,
		"Pod":                   podConditions,
		"PersistentVolumeClaim": pvcConditions,
	},
	"apps": map[string]GetConditionsFn{
		"StatefulSet": stsConditions,
		"DaemonSet":   daemonsetConditions,
		"Deployment":  deploymentConditions,
		"ReplicaSet":  replicasetConditions,
	},
	"policy": map[string]GetConditionsFn{
		"PodDisruptionBudget": pdbConditions,
	},
	"batch": map[string]GetConditionsFn{
		"CronJob": alwaysReady,
		"Job":     jobConditions,
	},
}

// GetLegacyReadyFn - True if we handle it as a known type
func GetLegacyReadyFn(u *unstructured.Unstructured) GetConditionsFn {
	gvk := u.GroupVersionKind()
	g := gvk.Group
	k := gvk.Kind
	if _, ok := legacyTypes[g]; ok {
		if fn, ok := legacyTypes[g][k]; ok {
			return fn
		}
	}
	return nil
}

func alwaysReady(u *unstructured.Unstructured) ([]Condition, error) {
	return []Condition{Condition{Type: ConditionReady, Reason: "always", Status: "True"}}, nil
}

// Statefulset
func stsConditions(u *unstructured.Unstructured) ([]Condition, error) {
	obj := u.UnstructuredContent()
	readyCondition := NewFalseCondition(ConditionReady)
	updateStrategy := clientu.GetStringField(obj, ".spec.updateStrategy.type", "")

	// updateStrategy==ondelete is a user managed statefulset.
	if updateStrategy == "ondelete" {
		readyCondition.Status = "True"
		readyCondition.Reason = "ondelete strategy"
		return []Condition{readyCondition}, nil
	}

	// ensure that the meta generation is observed
	observedGeneration := clientu.GetIntField(obj, ".status.observedGeneration", -1)
	metaGeneration := clientu.GetIntField(obj, ".metadata.generation", -1)
	if observedGeneration != metaGeneration {
		readyCondition.Reason = "Controller has not observed the latest change. Status generation does not match with metadata"
		return []Condition{readyCondition}, nil
	}

	// Replicas
	specReplicas := clientu.GetIntField(obj, ".spec.replicas", 1)
	readyReplicas := clientu.GetIntField(obj, ".status.readyReplicas", 0)
	currentReplicas := clientu.GetIntField(obj, ".status.currentReplicas", 0)
	updatedReplicas := clientu.GetIntField(obj, ".status.updatedReplicas", 0)
	statusReplicas := clientu.GetIntField(obj, ".status.replicas", 0)
	partition := clientu.GetIntField(obj, ".spec.updateStrategy.rollingUpdate.partition", -1)

	if specReplicas > statusReplicas {
		readyCondition.Reason = fmt.Sprintf("Waiting for requested replicas. Replicas: %d/%d", statusReplicas, specReplicas)
		return []Condition{readyCondition}, nil
	}

	if specReplicas > readyReplicas {
		readyCondition.Reason = fmt.Sprintf("Waiting for replicas to become Ready. Ready: %d/%d", readyReplicas, specReplicas)
		return []Condition{readyCondition}, nil
	}

	if partition != -1 {
		if updatedReplicas < (specReplicas - partition) {
			readyCondition.Reason = fmt.Sprintf("Waiting for partition rollout to complete. updated: %d/%d", updatedReplicas, specReplicas-partition)
		} else {
			// Partition case All ok
			readyCondition.Status = "True"
			readyCondition.Reason = fmt.Sprintf("Partition rollout complete. updated: %d", updatedReplicas)
		}
		return []Condition{readyCondition}, nil

	}

	if specReplicas > currentReplicas {
		readyCondition.Reason = fmt.Sprintf("Waiting for replicas to become current. current: %d/%d", currentReplicas, specReplicas)
		return []Condition{readyCondition}, nil
	}

	// Revision
	currentRevision := clientu.GetStringField(obj, ".status.currentRevision", "")
	updatedRevision := clientu.GetStringField(obj, ".status.updatedRevision", "")
	if currentRevision != updatedRevision {
		readyCondition.Reason = "Waiting for updated revision to match currentd"
		return []Condition{readyCondition}, nil
	}

	// All ok
	readyCondition.Status = "True"
	readyCondition.Reason = fmt.Sprintf("All replicas scheduled as expected. Replicas: %d", statusReplicas)
	return []Condition{readyCondition}, nil
}

// Deployment
func deploymentConditions(u *unstructured.Unstructured) ([]Condition, error) {
	obj := u.UnstructuredContent()
	readyCondition := NewFalseCondition(ConditionReady)

	progress := false
	available := false

	// ensure that the meta generation is observed
	observedGeneration := clientu.GetIntField(obj, ".status.observedGeneration", -1)
	metaGeneration := clientu.GetIntField(obj, ".metadata.generation", -1)
	if observedGeneration != metaGeneration {
		readyCondition.Reason = "Controller has not observed the latest change. Status generation does not match with metadata"
		return []Condition{readyCondition}, nil
	}

	conditions := clientu.GetConditions(obj)

	for _, c := range conditions {
		status := clientu.GetStringField(c, ".status", "")
		reason := clientu.GetStringField(c, ".reason", "")
		switch clientu.GetStringField(c, ".type", "") {
		case "Progressing": //appsv1.DeploymentProgressing:
			// https://github.com/kubernetes/kubernetes/blob/a3ccea9d8743f2ff82e41b6c2af6dc2c41dc7b10/pkg/controller/deployment/progress.go#L52
			if reason == "ProgressDeadlineExceeded" {
				readyCondition.Reason = "Progress Deadline exceeded"
				return []Condition{readyCondition}, nil
			}
			if status == "True" && reason == "NewReplicaSetAvailable" {
				progress = true
			}
		case "Available": //appsv1.DeploymentAvailable:
			if status == "True" {
				available = true
			}
		}
	}

	// replicas
	specReplicas := clientu.GetIntField(obj, ".spec.replicas", 1)
	statusReplicas := clientu.GetIntField(obj, ".status.replicas", 0)
	updatedReplicas := clientu.GetIntField(obj, ".status.updatedReplicas", 0)
	readyReplicas := clientu.GetIntField(obj, ".status.readyReplicas", 0)
	availableReplicas := clientu.GetIntField(obj, ".status.availableReplicas", 0)

	// TODO spec.replicas zero case ??

	if specReplicas > updatedReplicas {
		readyCondition.Reason = fmt.Sprintf("Waiting for all replicas to be updated. Updated: %d/%d", updatedReplicas, specReplicas)
		return []Condition{readyCondition}, nil
	}

	if statusReplicas > updatedReplicas {
		readyCondition.Reason = fmt.Sprintf("Waiting for old replicas to finish termination. Pending termination: %d", statusReplicas-updatedReplicas)
		return []Condition{readyCondition}, nil
	}

	if updatedReplicas > availableReplicas {
		readyCondition.Reason = fmt.Sprintf("Waiting for all replicas to be available. Available: %d/%d", availableReplicas, updatedReplicas)
		return []Condition{readyCondition}, nil
	}

	if specReplicas > readyReplicas {
		readyCondition.Reason = fmt.Sprintf("Waiting for all replicas to be ready. Ready: %d/%d", readyReplicas, specReplicas)
		return []Condition{readyCondition}, nil
	}

	if specReplicas > statusReplicas {
		readyCondition.Reason = fmt.Sprintf("Waiting for all .status.replicas to be catchup. replicas: %d/%d", statusReplicas, specReplicas)
		return []Condition{readyCondition}, nil
	}
	// check conditions
	if !progress {
		readyCondition.Reason = "New ReplicaSet is not available"
		return []Condition{readyCondition}, nil
	}
	if !available {
		readyCondition.Reason = "Deployment is not Available"
		return []Condition{readyCondition}, nil
	}
	// All ok
	readyCondition.Status = "True"
	readyCondition.Reason = fmt.Sprintf("Deployment is available. Replicas: %d", statusReplicas)
	return []Condition{readyCondition}, nil
}

// Replicaset
func replicasetConditions(u *unstructured.Unstructured) ([]Condition, error) {
	obj := u.UnstructuredContent()
	readyCondition := NewFalseCondition(ConditionReady)

	// ensure that the meta generation is observed
	observedGeneration := clientu.GetIntField(obj, ".status.observedGeneration", -1)
	metaGeneration := clientu.GetIntField(obj, ".metadata.generation", -1)
	if observedGeneration != metaGeneration {
		readyCondition.Reason = "Controller has not observed the latest change. Status generation does not match with metadata"
		return []Condition{readyCondition}, nil
	}

	// Conditions
	conditions := clientu.GetConditions(u.UnstructuredContent())
	for _, c := range conditions {
		switch clientu.GetStringField(c, ".type", "") {
		// https://github.com/kubernetes/kubernetes/blob/a3ccea9d8743f2ff82e41b6c2af6dc2c41dc7b10/pkg/controller/replicaset/replica_set_utils.go
		case "ReplicaFailure": //appsv1.ReplicaSetReplicaFailure
			status := clientu.GetStringField(c, ".status", "")
			if status == "True" {
				readyCondition.Reason = "Replica Failure condition. Check Pods"
				return []Condition{readyCondition}, nil
			}
		}
	}

	// Replicas
	specReplicas := clientu.GetIntField(obj, ".spec.replicas", 1)
	statusReplicas := clientu.GetIntField(obj, ".status.replicas", 0)
	readyReplicas := clientu.GetIntField(obj, ".status.readyReplicas", 0)
	availableReplicas := clientu.GetIntField(obj, ".status.availableReplicas", 0)
	labelledReplicas := clientu.GetIntField(obj, ".status.labelledReplicas", 0)

	if specReplicas == 0 && labelledReplicas == 0 && availableReplicas == 0 && readyReplicas == 0 {
		readyCondition.Reason = "Replica is 0"
		return []Condition{readyCondition}, nil
	}

	if specReplicas > labelledReplicas {

		readyCondition.Reason = fmt.Sprintf("Waiting for all replicas to be fully-labeled. Labelled: %d/%d", labelledReplicas, specReplicas)
		return []Condition{readyCondition}, nil
	}

	if specReplicas > availableReplicas {
		readyCondition.Reason = fmt.Sprintf("Waiting for all replicas to be available. Available: %d/%d", availableReplicas, specReplicas)
		return []Condition{readyCondition}, nil
	}

	if specReplicas > readyReplicas {
		readyCondition.Reason = fmt.Sprintf("Waiting for all replicas to be ready. Ready: %d/%d", readyReplicas, specReplicas)
		return []Condition{readyCondition}, nil
	}

	// All ok
	readyCondition.Status = "True"
	readyCondition.Reason = fmt.Sprintf("ReplicaSet is available. Replicas: %d", statusReplicas)
	return []Condition{readyCondition}, nil
}

// Daemonset
func daemonsetConditions(u *unstructured.Unstructured) ([]Condition, error) {
	obj := u.UnstructuredContent()
	readyCondition := NewFalseCondition(ConditionReady)

	// ensure that the meta generation is observed
	observedGeneration := clientu.GetIntField(obj, ".status.observedGeneration", -1)
	metaGeneration := clientu.GetIntField(obj, ".metadata.generation", -1)
	if observedGeneration != metaGeneration {
		readyCondition.Reason = "Controller has not observed the latest change. Status generation does not match with metadata"
		return []Condition{readyCondition}, nil
	}

	// replicas
	desiredNumberScheduled := clientu.GetIntField(obj, ".status.desiredNumberScheduled", -1)
	currentNumberScheduled := clientu.GetIntField(obj, ".status.currentNumberScheduled", 0)
	updatedNumberScheduled := clientu.GetIntField(obj, ".status.updatedNumberScheduled", 0)
	numberAvailable := clientu.GetIntField(obj, ".status.numberAvailable", 0)
	numberReady := clientu.GetIntField(obj, ".status.numberReady", 0)

	if desiredNumberScheduled == -1 {
		readyCondition.Reason = "Missing .status.desiredNumberScheduled"
		return []Condition{readyCondition}, nil
	}

	if desiredNumberScheduled > currentNumberScheduled {
		readyCondition.Reason = fmt.Sprintf("Waiting for desired replicas to be scheduled. Current: %d/%d", currentNumberScheduled, desiredNumberScheduled)
		return []Condition{readyCondition}, nil
	}

	if desiredNumberScheduled > updatedNumberScheduled {
		readyCondition.Reason = fmt.Sprintf("Waiting for updated replicas to be scheduled. Updated: %d/%d", updatedNumberScheduled, desiredNumberScheduled)
		return []Condition{readyCondition}, nil
	}

	if desiredNumberScheduled > numberAvailable {
		readyCondition.Reason = fmt.Sprintf("Waiting for replicas to be available. Available: %d/%d", numberAvailable, desiredNumberScheduled)
		return []Condition{readyCondition}, nil
	}

	if desiredNumberScheduled > numberReady {
		readyCondition.Reason = fmt.Sprintf("Waiting for replicas to be ready. Ready: %d/%d", numberReady, desiredNumberScheduled)
		return []Condition{readyCondition}, nil
	}

	// All ok
	readyCondition.Status = "True"
	readyCondition.Reason = fmt.Sprintf("All replicas scheduled as expected. Replicas: %d", desiredNumberScheduled)
	return []Condition{readyCondition}, nil
}

// PVC
func pvcConditions(u *unstructured.Unstructured) ([]Condition, error) {
	obj := u.UnstructuredContent()
	readyCondition := NewFalseCondition(ConditionReady)
	phase := clientu.GetStringField(obj, ".status.phase", "unknown")
	if phase != "Bound" { // corev1.ClaimBound
		readyCondition.Reason = fmt.Sprintf("PVC is not Bound. phase: %s", phase)
		return []Condition{readyCondition}, nil
	}
	// All ok
	readyCondition.Status = "True"
	readyCondition.Reason = "PVC is Bound"
	return []Condition{readyCondition}, nil
}

// Pod
func podConditions(u *unstructured.Unstructured) ([]Condition, error) {
	rv := []Condition{}
	readyCondition := NewFalseCondition(ConditionReady)
	obj := u.UnstructuredContent()

	phase := clientu.GetStringField(obj, ".status.phase", "unknown")
	conditions := clientu.GetConditions(obj)

	for _, c := range conditions {
		switch clientu.GetStringField(c, ".type", "") {
		case "Ready":
			readyCondition.Reason = clientu.GetStringField(c, "reason", "")
			if clientu.GetStringField(c, "status", "") == "True" {
				readyCondition.Status = "True"
			} else {
				readyCondition.Status = "False"
				if clientu.GetStringField(c, "reason", "") == "PodCompleted" {
					readyCondition.Status = "True"
					if phase == "Succeeded" {
						rv = append(rv, NewCondition(ConditionCompleted, "Pod Succeeded").Get())
					} else {
						rv = append(rv, NewCondition(ConditionFailed, fmt.Sprintf("Pod phase: %s", phase)).Get())
					}
				}
			}
		}
	}

	if readyCondition.Reason == "" {
		readyCondition.Reason = "Phase: " + phase
	} else {
		readyCondition.Reason = "Phase: " + phase + ", " + readyCondition.Reason
	}
	rv = append(rv, readyCondition)
	return rv, nil
}

// PodDisruptionBudget
func pdbConditions(u *unstructured.Unstructured) ([]Condition, error) {
	obj := u.UnstructuredContent()
	readyCondition := NewFalseCondition(ConditionReady)

	// replicas
	currentHealthy := clientu.GetIntField(obj, ".status.currentHealthy", 0)
	desiredHealthy := clientu.GetIntField(obj, ".status.desiredHealthy", -1)
	if desiredHealthy == -1 {
		readyCondition.Reason = "Missing .status.desiredHealthy"
		return []Condition{readyCondition}, nil
	}
	if desiredHealthy > currentHealthy {
		readyCondition.Reason = fmt.Sprintf("Budget not met. healthy replicas: %d/%d", currentHealthy, desiredHealthy)
		return []Condition{readyCondition}, nil
	}

	// All ok
	readyCondition.Status = "True"
	readyCondition.Reason = fmt.Sprintf("Budget is met. Replicas: %d/%d", currentHealthy, desiredHealthy)
	return []Condition{readyCondition}, nil
}

// Job
func jobConditions(u *unstructured.Unstructured) ([]Condition, error) {
	obj := u.UnstructuredContent()

	parallelism := clientu.GetIntField(obj, ".spec.parallelism", 1)
	completions := clientu.GetIntField(obj, ".spec.completions", parallelism)
	succeeded := clientu.GetIntField(obj, ".status.succeeded", 0)
	active := clientu.GetIntField(obj, ".status.active", 0)
	failed := clientu.GetIntField(obj, ".status.failed", 0)
	conditions := clientu.GetConditions(obj)
	starttime := clientu.GetStringField(obj, ".status.startTime", "")

	// Conditions
	// https://github.com/kubernetes/kubernetes/blob/master/pkg/controller/job/utils.go#L24
	for _, c := range conditions {
		status := clientu.GetStringField(c, ".status", "")
		switch clientu.GetStringField(c, ".type", "") {
		case "Complete":
			if status == "True" {
				message := fmt.Sprintf("Job Completed. succeded: %d/%d", succeeded, completions)
				return []Condition{
					NewCondition(ConditionReady, message).Get(),
					NewCondition(ConditionCompleted, message).Get(),
				}, nil
			}
		case "Failed":
			if status == "True" {
				message := fmt.Sprintf("Job Failed. failed: %d/%d", failed, completions)
				return []Condition{
					NewCondition(ConditionReady, message).Get(),
					NewCondition(ConditionFailed, message).Get(),
				}, nil
			}
		}
	}

	// replicas
	if starttime == "" {
		message := "Job not started"
		return []Condition{
			NewCondition(ConditionReady, message).False().Get(),
		}, nil
	}
	message := fmt.Sprintf("Job in progress. success:%d, active: %d, failed: %d", succeeded, active, failed)
	return []Condition{
		NewCondition(ConditionReady, message).Get(),
	}, nil
}

// Service
func serviceConditions(u *unstructured.Unstructured) ([]Condition, error) {
	obj := u.UnstructuredContent()

	specType := clientu.GetStringField(obj, ".spec.type", "ClusterIP")
	specClusterIP := clientu.GetStringField(obj, ".spec.clusterIP", "")
	//statusLBIngress := clientu.GetStringField(obj, ".status.loadBalancer.ingress", "")

	message := fmt.Sprintf("Always Ready. Service type: %s", specType)
	if specType == "LoadBalancer" {
		if specClusterIP == "" {
			message = "ClusterIP not set. Service type: LoadBalancer"
			return []Condition{
				NewCondition(ConditionReady, message).False().Get(),
			}, nil
		}
		message = fmt.Sprintf("ClusterIP: %s", specClusterIP)
	}

	return []Condition{
		NewCondition(ConditionReady, message).Get(),
	}, nil
}
