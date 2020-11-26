package res

type ReadyProbe interface {
	IsReady(config KubeReadyConfiguration) (ok bool, err error)
	Diagnosis(config KubeReadyConfiguration) string
}
type ProbeResult struct {
	Result bool
	Err    error
	PodName string
	PodStatus PodEnumStatus
}
