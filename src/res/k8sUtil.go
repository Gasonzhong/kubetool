package res

import (
	"encoding/json"
	"errors"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/watch"
	"sync"
	"time"
)

type PodEnumStatus int

var (
	mu          sync.Mutex // guards balance
	isWatchPods = false
)

const (
	_PodEnumStatus = iota
	SUCCESS
	READY
	FAILURE
	TERMINATED
)

func WaitAndKillWatch(wi watch.Interface, tmout time.Duration) {
	<-time.After(tmout)
	wi.Stop()
}
func WaitAndCloseChannel(ch chan ProbeResult, tmout time.Duration) {
	DebugLog("timeout-total:", tmout)
	<-time.After(tmout)
	ch <- ProbeResult{false, errors.New("timeout and channel close"), "", FAILURE}
}

/**
判读部署是否失败
 */
func IsDeloyFail(replicas int32, fail int32, successThreshold int32) bool {
	DebugLog("replicas:", replicas)
	DebugLog("failPod:", fail)
	DebugLog("successThreshold:", successThreshold)
	return fail*100 > ((100 - successThreshold) * replicas)
	//return (replicas-failPod)/replicas*100 > (100 - successThreshold)
}

/**
判读部署是否成功
 */
func IsDeloySuccess(replicas int32, successPod int32, successThreshold int32) bool {
	DebugLog("replicas:", replicas)
	DebugLog("successPod:", successPod)
	DebugLog("successThreshold:", successThreshold)
	return successPod*100/replicas >= successThreshold
}
func ConvertByJson(in interface{}, out interface{}) (err error) {
	err = nil
	var data []byte
	data, err = json.Marshal(in)
	if err != nil {
		return
	}
	err = json.Unmarshal(data, &out)
	return
}

func CheckReadyPod(pod core_v1.Pod, config *KubeReadyConfiguration) (PodEnumStatus) {
	DebugLog("check pod:", pod.Name)
	/**
		  restartCount 小于 *containerRestartLimit 失败
		  启动中:其中所有的容器状态:state == "running" || state == ""waitting" && state.reason == "PodInitializing"
	 */
	if pod.Status.Phase == "Failed" || pod.Status.Phase == "Unknown" {
		return FAILURE
	}
	for _, containerStatus := range pod.Status.ContainerStatuses {
		if containerStatus.RestartCount > int32(config.ContainerRestartLimit) {
			DebugLog("RestartCount>ContainerRestartLimit:", true)
			DebugLog("podStatus:", "FAILURE")
			return FAILURE
		}
		if containerStatus.State.Terminated != nil {
			return TERMINATED
		}
		if containerStatus.State.Waiting != nil && containerStatus.State.Waiting.Reason == "PodInitializing" {
			DebugLog("containerStatus.state.waiting.reason: ", containerStatus.State.Waiting.Reason)
			continue
		}
		if containerStatus.State.Waiting != nil && containerStatus.State.Waiting.Reason == "ContainerCreating" {
			DebugLog("containerStatus.state.waiting.reason: ", containerStatus.State.Waiting.Reason)
			continue
		}
		if containerStatus.State.Running != nil {
			DebugLog("containerStatus.state.running.startedAt: ", containerStatus.State.Running.StartedAt)
			continue
		}
		DebugLog("podStatus-containerStatus:", "FAILURE")
		return FAILURE
	}
	if getCondition("Ready", pod, config) {
		return SUCCESS
	}
	if getCondition("Initialized", pod, config) {
		return READY
	}
	return FAILURE
}

func getCondition(conditionType string, pod core_v1.Pod, config *KubeReadyConfiguration) (bool) {
	startTime := pod.Status.StartTime.UTC().Unix()
	for _, condition := range pod.Status.Conditions {
		if conditionType == "Ready" && condition.Type == "Ready" {
			if condition.Status == "True" {
				DebugLog("podStatus-conditon:", "SUCCESS")
				return true
			}
		}
		/*   成功:status.conditions[type==Ready].status=="True"   是否是严格标准？？？*/

		/**   启动中:
		  初始化时间未超时 status == "False"  && (lastTransitionTime - Pod.sta   tus.startTime < *pendingTimeout)
		or
		   探针时间未超时status == "True" &&   当前时间(使用修正后的引擎pod时间->master时间和pod时间) -  lastTransitionTime < *probeTimeout
		**/
		// 初始化时间未超时 pod.Status.StartTime
		if condition.Type == "Initialized" && conditionType == "Initialized" {
			if pod.Status.StartTime == nil {
				return false
			}
			lastTransitionTime := condition.LastTransitionTime.UTC().Unix()
			if condition.Status == "False" && (lastTransitionTime-startTime < config.PendingTimeout) {
				DebugLog("podStatus:", "READY")
				return true
			}
			now := time.Now().UTC().Unix() //当前时间。k8中是0时区，本地调试时要注意
			if condition.Status == "True" && (now-lastTransitionTime < config.ProbeTimeout) {
				DebugLog("podStatus:", "READY")
				return true
			}
		}
	}
	return false
}

func LimitString(str string, maxlen int) (ret string) {
	ret = str
	rs := []rune(ret)
	rslen := len(rs)
	if rslen > maxlen {
		rs = rs[rslen-maxlen:]
	}
	ret = string(rs)
	return
}

//监视pods开启
func WatchingPods() {
	//加锁
	mu.Lock()
	isWatchPods = true
	//解锁
	mu.Unlock()
}

// 是否已经监视
func IsWatching() bool {
	//加锁
	mu.Lock()
	b := isWatchPods
	//解锁
	mu.Unlock()
	return b
}
