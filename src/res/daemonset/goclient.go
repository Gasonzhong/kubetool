package daemonset

import (
	"context"
	"errors"
	"fmt"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"kubetool/src/res"
	"kubetool/src/res/deployment"
	"strings"
	"time"
)

type ClientDaemonsetProbe struct {
	deployment.ClientDeploymentReadyProbe
}

func NewClientDaemonsetProbe() ClientDaemonsetProbe {
	ret := ClientDaemonsetProbe{}
	ret.ResultChannel = make(chan res.ProbeResult)
	return ret
}

func (this ClientDaemonsetProbe) IsReady(config res.KubeReadyConfiguration) (ok bool, err error) {
	// ctx, cancel := context.WithCancel(context.Background())
	// defer cancel()
	ok = false
	err = nil
	this.Config = &config
	var client *kubernetes.Clientset
	client, err = this.GetK8sClient(config)
	if err != nil {
		return
	}
	this.ReadyPod = 0
	this.SuccessPod = 0
	this.Client = client
	lo := v1.ListOptions{}
	lo.FieldSelector = "metadata.name=" + config.ResourceName
	// 获取 DaemonSets
	daemonset, err := client.AppsV1().DaemonSets(config.NameSpace).Get(context.TODO(), config.ResourceName, v1.GetOptions{})
	if err != nil {
		return false, err
	}
	res.DebugLog("当前代：", daemonset.Labels["pod-template-generation"])
	/**
	  daemonset.Spec.UpdateStrategy.Type  "RollingUpdate" or "OnDelete". Default is RollingUpdate
	 */
	this.PodStatusMap, err = this.CheckDepPodsBeforeWatch(daemonset.Labels["pod-template-generation"], daemonset.Spec.Selector.MatchLabels, this.Config)
	// 初始化状态
	this.SuccessPod, this.ReadyPod, this.FailPod, this.Terminated = this.ChangeNum(this.PodStatusMap)

	// 期望值
	this.Replicas = daemonset.Status.DesiredNumberScheduled
	//var deployW watch.Interface
		//// 探针
	//deployW, err = client.AppsV1().DaemonSets(config.NameSpace).Watch(context.TODO(), lo)
	//if err != nil {
	//	return false, err
	//}
	//go res.WaitAndKillWatch(deployW, time.Second*time.Duration(config.ProbeTimeout))
	//go this.WatchDaemonset(deployW.ResultChan())

	go this.WatchPods(*daemonset)
	// 总时间超时
	go res.WaitAndCloseChannel(this.ResultChannel, time.Second*time.Duration(config.ProbeTimeout+config.PendingTimeout))

	for {
		result := <-this.ResultChannel
		// 已经部署成功的情况
		if result.Result {
			return result.Result, result.Err
		}
		// 错误，超时等情况
		if result.Err != nil {
			return result.Result, result.Err
		}
		// pod信息
		if result.PodName != "" {
			this.PodStatusMap[result.PodName] = result.PodStatus
			this.SuccessPod, this.ReadyPod, this.FailPod, this.Terminated = this.ChangeNum(this.PodStatusMap)
			res.DebugLog("name:", result.PodName, "status:", this.PodStatusMap[result.PodName])
			res.DebugLog("ready:", this.ReadyPod, "success:", this.SuccessPod)
		}
		// 滚升时，第一个就错误
		if this.ReadyPod == 0 && this.SuccessPod == 0 && this.FailPod > 0 {
			return false, errors.New("daemonset updating is failed")
		}
		// 失败
		if res.IsDeloyFail(this.Replicas, this.FailPod, int32(this.Config.SuccThreshold)) {
			return false, errors.New("daemonset is failed")
		}
		// 成功
		if res.IsDeloySuccess(this.Replicas, this.SuccessPod, int32(this.Config.SuccThreshold)) {
			return true, nil
		}
	}
	return false, errors.New("error")
}

func (this ClientDaemonsetProbe) WatchDaemonset(ch <-chan watch.Event) {
	res.DebugLog("start watch daemonset")
	for {
		e, ok := <-ch
		if !ok {
			res.DebugLog("deployment watch channel closed")
			this.ResultChannel <- res.ProbeResult{false, errors.New("daemonset watch channel closed"), "", res.FAILURE}
			return
		}
		if e.Type == watch.Added || e.Type == watch.Modified {

			// res.DebugLog(e.Object)

			var de appv1.DaemonSet
			err := res.ConvertByJson(e.Object, &de)
			if err != nil {
				return
			}
			res.DebugLog(res.ToJson(de))
			if !res.IsWatching() {
				res.WatchingPods()
				go this.WatchPods(de)
			}

		}
	}

}

func (this ClientDaemonsetProbe) CheckDepPodsBeforeWatch(revision string, labels map[string]string, config *res.KubeReadyConfiguration) (podMap map[string]res.PodEnumStatus, err error) {
	labelSelector := ""
	for key, value := range labels {
		labelSelector = labelSelector + key + "=" + value + ","
	}
	labelSelector = strings.TrimRight(labelSelector, ",")
	podList, err := this.Client.CoreV1().Pods(config.NameSpace).List(context.TODO(), v1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	podMap = make(map[string]res.PodEnumStatus)
	for _, pod := range podList.Items {
		res.DebugLog("pod的版本", pod.Labels["controller-revision-hash"])
		// 当前代系
		if pod.Labels["controller-revision-hash"] == revision {
			podMap[pod.Name] = res.CheckReadyPod(pod, config)
			res.DebugLog("pod.name: ", pod.Name)
		}
	}
	return podMap, nil
}

func (this ClientDaemonsetProbe) ValidateDaemonset(daemonset appv1.DaemonSet) bool {
	return daemonset.Status.DesiredNumberScheduled == daemonset.Status.NumberReady

}

func (this ClientDaemonsetProbe) WatchPods(deployment appv1.DaemonSet) {
	labelSelector := ""
	for key, value := range deployment.Spec.Selector.MatchLabels {
		labelSelector = labelSelector + key + "=" + value + ","
	}
	labelSelector = strings.TrimRight(labelSelector, ",")

	// 先获取已经成功的
	//this.CheckPods(labelSelector)
	wi, err := this.Client.CoreV1().Pods(this.Config.NameSpace).Watch(context.TODO(), v1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		res.DebugLog("watch pods error:", err)
		this.ResultChannel <- res.ProbeResult{false, err, "", res.FAILURE}
		return
	}
	go res.WaitAndKillWatch(wi, time.Second*time.Duration(this.Config.ProbeTimeout))
	for {
		we, ok := <-wi.ResultChan()
		if !ok {
			res.DebugLog("pod watch channel closed")
			return
		}
		res.DebugLog("---WatchPods---", we.Type)
		if we.Type == watch.Added || we.Type == watch.Modified {
			res.DebugLog("WatchPods", we.Type)
			var podData corev1.Pod
			res.ConvertByJson(we.Object, &podData)
			if podData.Labels["controller-revision-hash"] != deployment.Labels["pod-template-generation"] {
				continue
			}
			// 判断pod状态
			podStatus := res.CheckReadyPod(podData, this.Config)
			this.ResultChannel <- res.ProbeResult{false, nil, podData.Name, podStatus}

		}
	}

}
func (this ClientDaemonsetProbe) Diagnosis(config res.KubeReadyConfiguration) string {
	this.Config = &config
	var err error
	this.Client, err = this.GetK8sClient(config)
	if err != nil {

		return err.Error()
	}
	ret := make([]string, 0, 10)
	ret = this.AppendIfNotEmpty(ret, this.DaemonsetDiagnosis())

	return strings.Join(ret, "\n")

}

func (this ClientDaemonsetProbe) DaemonsetDiagnosis() string {
	res.DebugLog("dd")
	ggo := v1.GetOptions{}
	de, err := this.Client.AppsV1().DaemonSets(this.Config.NameSpace).Get(context.TODO(), this.Config.ResourceName, ggo)
	if err != nil {
		return err.Error()
	}
	lo := v1.ListOptions{}
	lo.Limit = 100
	uiStr := string(de.GetObjectMeta().GetUID())
	se := this.Client.CoreV1().Events("").GetFieldSelector(nil, nil, nil, &uiStr)
	lo.FieldSelector = se.String()
	res.DebugLog(se.String())

	events, err := this.Client.CoreV1().Events(this.Config.NameSpace).List(context.TODO(), lo)
	if err != nil {
		return err.Error()
	}
	lines := make([]string, len(events.Items))
	lines = append(lines, de.GetObjectMeta().GetSelfLink()+"/events")
	for _, e := range events.Items {
		lines = append(lines, e.Type+"  "+e.Reason+"  "+res.LimitString(e.Message, 100)+"  "+e.GetObjectMeta().GetCreationTimestamp().Format("Mon Jan 2 15:04:05 -0700 MST 2006"))
		res.DebugLog(res.ToJson(e))
	}
	lines = append(lines, "---------")
	lines = append(lines, this.PodDiagnosis(string(de.GetObjectMeta().GetUID())))
	return strings.Join(lines, "\n")
}
