package api

type Deployment struct {
	ReplicaSet
}
type ReplicaSet struct {
	Kind     string            `json:"kind"`
	Metadata Metadata          `json:"metadata"`
	Status   ReplicatSetStatus `json:"status"`
}
type Pod struct {
	Kind     string    `json:"kind"`
	Metadata Metadata  `json:"metadata"`
	Status   PodStatus `json:"status"`
}
type StatefulSet struct {
	Kind     string            `json:"kind"`
	Metadata Metadata          `json:"metadata"`
	Status   StatefulSetStatus `json:"status"`
}
type DaemonSet struct {
	Kind     string          `json:"kind"`
	Metadata Metadata        `json:"metadata"`
	Status   DaemonSetStatus `json:"status"`
}

type Metadata struct {
	Generation      int         `json:"generation"`
	Name            string      `json:"name"`
	Namespace       string      `json:"namespace`
	OwnerReferences []Reference `json:"ownerReferences"`
	Annotations     Annotations `json:"annotations"`
}
type Reference struct {
	ApiVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Uid        string `json:"uid"`
}
type ReplicatSetStatus struct {
	AvailableReplicas    int                          `json:"availableReplicas"`
	FullyLabeledReplicas int                          `json:"fullyLabeledReplicas"`
	ObservedGeneration   int                          `json:"observedGeneration"`
	ReadyReplicas        int                          `json:"readyReplicas"`
	Replicas             int                          `json:"replicas"`
	Conditions           []ReplicatSetStatusCondition `json:"conditions"`
}
type PodStatus struct {
	HostIP            string            `json:"hostIP"`
	Phase             string            `json:"phase"`
	StartTime         string            `json:"startTime"`
	ContainerStatuses []ContainerStatus `json:"containerStatuses"`
}

type List struct {
	Items []interface{} `json:"items"`
}

type ContainerStatus struct {
	Name         string                       `json:"name"`
	Ready        bool                         `json:"ready"`
	RestartCount int                          `json:"restartCount"`
	State        map[string]map[string]string `json:"state"`
}

type Annotations map[string]string

type StatefulSetStatus struct {
	CurrentReplicas    int                          `json:"currentReplicas"`
	ObservedGeneration int                          `json:"observedGeneration"`
	ReadyReplicas      int                          `json:"readyReplicas"`
	Replicas           int                          `json:"replicas"`
	Conditions         []ReplicatSetStatusCondition `json:"conditions"`
}

type ReplicatSetStatusCondition struct {
	Message string `json:"message"`
	Reason  string `json:"reason"`
	Status  string `json:"status"`
	Type    string `json:"type"`
}

type DaemonSetStatus struct {
	DesiredNumberScheduled int `json:"desiredNumberScheduled"`
	NumberReady            int `json:"numberReady"`
}
