package deployment

import (
	"context"
	"errors"
	"fmt"
	"kubetool/src/res"
	"strings"
	"time"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/util/wait"
	discovery "k8s.io/apimachinery/pkg/version"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type ClientDeploymentReadyProbe struct {
	ResultChannel chan res.ProbeResult
	Client        *kubernetes.Clientset
	Config        *res.KubeReadyConfiguration
	PodStatusMap  map[string]res.PodEnumStatus
	IsUpdate      bool
	FailPod       int32
	ReadyPod      int32
	SuccessPod    int32
	Terminated    int32
	Replicas      int32
}

func NewClientDeploymentReadyProbe() ClientDeploymentReadyProbe {
	return ClientDeploymentReadyProbe{ResultChannel: make(chan res.ProbeResult)}
}

func (this ClientDeploymentReadyProbe) IsReady(config res.KubeReadyConfiguration) (ok bool, err error) {
	ok = false
	err = nil
	this.Config = &config
	var client *kubernetes.Clientset
	client, err = this.GetK8sClient(config)
	if err != nil {
		return
	}
	this.Client = client
	lo := v1.ListOptions{}
	lo.FieldSelector = "metadata.name=" + config.ResourceName
	// 获取deployment
	deployment, err := client.AppsV1().Deployments(config.NameSpace).Get(context.TODO(), config.ResourceName, v1.GetOptions{})
	if err != nil {
		return false, err
	}
	reps, err := this.FindReplica(deployment)
	if err != nil {
		return false, err
	}
	this.PodStatusMap, err = this.CheckDepPodsBeforeWatch(reps.Labels["pod-template-hash"], deployment.Spec.Selector.MatchLabels, this.Config)

	//this.IsUpdate = isUpdate(*deployment)
	// 初始化状态
	/**
	deployment.Status.Conditions[type==Progressing] status==true
	   1.reason==NewReplicaSetCreated 创建
       2.reason==ReplicaSetUpdated  滚升
	   deployment.Spec.Strategy.Type=="RollingUpdate" 滚动更新
	   deployment.Spec.Strategy.Type=="Recreate" 全量更新
	 */
	this.SuccessPod, this.ReadyPod, this.FailPod, this.Terminated = this.ChangeNum(this.PodStatusMap)

	res.DebugLog(this.SuccessPod, " has success", )
	// 期望值
	this.Replicas = *deployment.Spec.Replicas - this.Terminated
	//var deployW watch.Interface
	//// 探针
	//deployW, err = client.AppsV1().Deployments(config.NameSpace).Watch(context.TODO(), lo)
	//if err != nil {
	//	return false, err
	//}
	//go res.WaitAndKillWatch(deployW, time.Second*time.Duration(config.ProbeTimeout))
	//go this.WatchDeployment(deployW.ResultChan())
	go this.WatchPods(reps.Labels["pod-template-hash"], *deployment)
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
			return false, errors.New("deployment updating is failed")
		}

		// 失败
		if res.IsDeloyFail(this.Replicas, this.FailPod, int32(this.Config.SuccThreshold)) {
			return false, errors.New("deployment is failed")
		}
		// 成功
		if res.IsDeloySuccess(this.Replicas, this.SuccessPod, int32(this.Config.SuccThreshold)) {
			return true, nil
		}
	}
	return false, errors.New("error")
}

func isUpdate(deployment appv1.Deployment) (bool) {
	for _, condition := range deployment.Status.Conditions {
		if condition.Type == "Progressing" && condition.Status == "ReplicaSetUpdated" {
			return true
		}
	}
	return false
}

/**

 */
func (this ClientDeploymentReadyProbe) ChangeNum(statusMap map[string]res.PodEnumStatus) (success int32, ready int32, fail int32, terminated int32) {
	success = 0
	ready = 0
	fail = 0
	terminated = 0
	for _, v := range this.PodStatusMap {
		if v == res.SUCCESS {
			success++
		}
		if v == res.READY {
			ready++
		}
		if v == res.FAILURE {
			fail++
		}
		if v == res.TERMINATED {
			terminated++
		}
	}
	return success, ready, fail, terminated
}

/**
   在watch之前检查pod是否有成功的
 */
func (this ClientDeploymentReadyProbe) CheckDepPodsBeforeWatch(revision string, labels map[string]string, config *res.KubeReadyConfiguration) (podMap map[string]res.PodEnumStatus, err error) {
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
		if revision == pod.Labels["pod-template-hash"] {
			podMap[pod.Name] = res.CheckReadyPod(pod, config)
			res.DebugLog("pod.name: ", pod.Name)
		}
	}
	return podMap, nil
}

func (this ClientDeploymentReadyProbe) Diagnosis(config res.KubeReadyConfiguration) string {
	this.Config = &config
	var err error
	this.Client, err = this.GetK8sClient(config)
	if err != nil {

		return err.Error()
	}
	ret := make([]string, 10)
	ret = this.AppendIfNotEmpty(ret, this.DeploymentDiagnosis())

	return strings.Join(ret, "\n")

}
func (this ClientDeploymentReadyProbe) AppendIfNotEmpty(arr []string, appendStr string) []string {
	if len(appendStr) > 0 {
		return append(arr, appendStr)
	}
	return arr
}

func (this ClientDeploymentReadyProbe) DeploymentDiagnosis() string {
	res.DebugLog("DeploymentDiagnosis")
	ggo := v1.GetOptions{}
	de, err := this.Client.AppsV1().Deployments(this.Config.NameSpace).Get(context.TODO(), this.Config.ResourceName, ggo)
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
	lines = append(lines, this.ReplicaSetDiagnosis(de))
	return strings.Join(lines, "\n")
}

func (this ClientDeploymentReadyProbe) ReplicaSetDiagnosis(d *appv1.Deployment) string {
	res.DebugLog("ReplicaSetDiagnosis")
	rs, err := this.FindReplica(d)
	if err != nil {
		return err.Error()
	}
	lo := v1.ListOptions{}
	lo.Limit = 100
	// lo.FieldSelector = "regarding.uid=" + string(de.GetObjectMeta().GetUID())
	// this.Client.CoreV1().Events()
	uiStr := string(rs.GetObjectMeta().GetUID())
	// kindStr := "Deployment"
	// nameStr := de.GetObjectMeta().GetName()
	se := this.Client.CoreV1().Events("").GetFieldSelector(nil, nil, nil, &uiStr)
	lo.FieldSelector = se.String()
	res.DebugLog(se.String())
	// events, err := this.Client.CoreV1().Events("").
	events, err := this.Client.CoreV1().Events(this.Config.NameSpace).List(context.TODO(), lo)
	if err != nil {
		return err.Error()
	}
	lines := make([]string, len(events.Items))
	lines = append(lines, rs.GetObjectMeta().GetSelfLink()+"/events")
	for _, e := range events.Items {
		lines = append(lines, e.Type+"  "+e.Reason+"  "+res.LimitString(e.Message, 100)+"  "+e.GetObjectMeta().GetCreationTimestamp().Format("Mon Jan 2 15:04:05 -0700 MST 2006"))
		res.DebugLog(res.ToJson(e))
	}
	lines = append(lines, "---------")
	lines = append(lines, this.PodDiagnosis(string(rs.GetObjectMeta().GetUID())))
	return strings.Join(lines, "\n")
}

/**
pod状态打印
 */
func (this ClientDeploymentReadyProbe) PodDiagnosis(puid string) string {
	res.DebugLog("PodDiagnosis:", puid)
	lo := v1.ListOptions{}

	pods, err := this.Client.CoreV1().Pods(this.Config.NameSpace).List(context.TODO(), lo)
	if err != nil {
		return err.Error()
	}
	lines := make([]string, 0)
	for _, pod := range pods.Items {
		refers := pod.GetObjectMeta().GetOwnerReferences()
		match := false
		for _, r := range refers {
			if string(r.UID) == puid {
				match = true
				break
			}
		}
		if !match {
			continue
		}

		lines = append(lines, pod.GetObjectMeta().GetSelfLink()+"/events")
		lo := v1.ListOptions{}
		lo.Limit = 100
		uiStr := string(pod.GetObjectMeta().GetUID())
		se := this.Client.CoreV1().Events("").GetFieldSelector(nil, nil, nil, &uiStr)
		lo.FieldSelector = se.String()
		res.DebugLog(se.String())
		events, err := this.Client.CoreV1().Events(this.Config.NameSpace).List(context.TODO(), lo)
		if err != nil {
			return err.Error()
		}
		for _, e := range events.Items {
			lines = append(lines, e.Type+"  "+e.Reason+"  "+res.LimitString(e.Message, 100)+"  "+e.GetObjectMeta().GetCreationTimestamp().Format("Mon Jan 2 15:04:05 -0700 MST 2006"))
			res.DebugLog(res.ToJson(e))
		}
		lines = append(lines, "---------")
		for _, container := range pod.Spec.Containers {

			logo := corev1.PodLogOptions{}
			var tl int64
			tl = 50
			logo.TailLines = &tl
			logo.Container = container.Name

			ls := this.Client.CoreV1().Pods(this.Config.NameSpace).GetLogs(pod.GetObjectMeta().GetName(), &logo)
			res.DebugLog(ls.URL().String())

			logData, err := ls.DoRaw(context.TODO())
			if err != nil {
				res.DebugLog(err)
			} else {
				lines = append(lines, pod.GetObjectMeta().GetSelfLink()+"/"+container.Name+"/logs")
				lines = append(lines, string(logData))
				lines = append(lines, "---------")
				res.DebugLog("to here")
			}

		}

	}
	return strings.Join(lines, "\n")

}

func (this ClientDeploymentReadyProbe) WatchDeployment(revison string,ch <-chan watch.Event) {
	res.DebugLog("WatchDeployment")
	for {
		e, ok := <-ch
		//  超时关闭错误
		if !ok {
			res.DebugLog("deployment watch channel closed")
			this.ResultChannel <- res.ProbeResult{false, errors.New("deployment watch channel closed"), "", res.FAILURE}
			return
		}
		if e.Type == watch.Added || e.Type == watch.Modified {

			res.DebugLog("WatchDeployment", e.Type)

			var de appv1.Deployment
			err := res.ConvertByJson(e.Object, &de)
			if err != nil {
				return
			}
			res.DebugLog(res.ToJson(de))
			// pod探针
			if !res.IsWatching() {
				res.WatchingPods()
				go this.WatchPods(revison,de)
			}

		}

	}
}

func (this ClientDeploymentReadyProbe) WatchPods(revision string, deployment appv1.Deployment) {
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
			// 判断pod状态
			res.DebugLog("pod-revision-:", podData.Labels["pod-template-hash"])
			if podData.Labels["pod-template-hash"] != revision {
				continue
			}
			podStatus := res.CheckReadyPod(podData, this.Config)
			this.ResultChannel <- res.ProbeResult{false, nil, podData.Name, podStatus}
		}
	}
}



func (this ClientDeploymentReadyProbe) CheckPod(pod corev1.Pod) (err error) {
	err = nil
	css := pod.Status.ContainerStatuses
	for _, cs := range css {
		wa := cs.State.Waiting
		if wa == nil {
			return
		}
		if wa.Reason == "CrashLoopBackOff" || wa.Reason == "ImagePullBackOff" || wa.Reason == "CreateContainerConfigError" || wa.Reason == "ContainerCannotRun" {
			return errors.New("container " + pod.GetObjectMeta().GetNamespace() + "/" + pod.GetObjectMeta().GetName() + "/" + cs.Name + " is " + wa.Reason)
		}

	}
	res.DebugLog(res.ToJson(pod))
	return

}

/**
通过时间戳
 */
func (this ClientDeploymentReadyProbe) FindReplica(deployment *appv1.Deployment) (ret appv1.ReplicaSet, err error) {
	res.DebugLog("FindReplica")
	lo := v1.ListOptions{}
	var maxGen int64 = -1
	err = nil

	rss, err := this.Client.AppsV1().ReplicaSets(this.Config.NameSpace).List(context.TODO(), lo)
	if err != nil {
		this.ResultChannel <- res.ProbeResult{false, err, "", res.FAILURE}
	}
	ret = rss.Items[0]
	for _, rs := range rss.Items {
		fmt.Println("rs-version", rs.Labels["pod-template-hash"])
		refers := rs.GetObjectMeta().GetOwnerReferences()
		for _, ref := range refers {
			if ref.UID == deployment.UID {
				maxGen = rs.GetGeneration()
				if rs.GetCreationTimestamp().UTC().Unix() > ret.GetCreationTimestamp().UTC().Unix() {
					ret = rs
				}
			}
		}
	}
	res.DebugLog(res.ToJson(ret))
	if maxGen == -1 {
		err = errors.New("replicatSets not found by deployment " + deployment.ObjectMeta.Name)
	} else {
		for _, rscd := range ret.Status.Conditions {
			if rscd.Type == "ReplicaFailure" && rscd.Status == corev1.ConditionTrue && rscd.Reason == "FailedCreate" {
				err = errors.New("ReplicaFailure")
				// this.ResultChannel <- res.ProbeResult{false, err}
				return ret, errors.New("fail")
			}

		}
	}

	return ret, nil

}

/**
判断是否dep成功
 */
func (this ClientDeploymentReadyProbe) ValidateDeployment(de appv1.Deployment) (result bool) {
	if de.Status.ReadyReplicas == *de.Spec.Replicas {
		res.DebugLog("ValidateDeployment:", true)
		return true
	}
	return false
}

func (this ClientDeploymentReadyProbe) GetK8sClient(config res.KubeReadyConfiguration) (*kubernetes.Clientset, error) {
	res.DebugLog("init k8s client from:" + config.KubeConfigPath)
	cfg, err := clientcmd.BuildConfigFromFlags("", config.KubeConfigPath)
	if err != nil {
		return nil, err
	}

	cfg.QPS = 0
	cfg.Burst = 0
	cfg.ContentType = "application/vnd.kubernetes.protobuf"

	res.DebugLog("Creating API client for ", cfg.Host)

	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	var v *discovery.Info

	// The client may fail to connect to the API server in the first request.
	// https://github.com/kubernetes/ingress-nginx/issues/1968
	defaultRetry := wait.Backoff{
		Steps:    10,
		Duration: 1 * time.Second,
		Factor:   1.5,
		Jitter:   0.1,
	}

	var lastErr error
	retries := 0
	res.DebugLog(fmt.Sprintf("Trying to discover Kubernetes version"))
	err = wait.ExponentialBackoff(defaultRetry, func() (bool, error) {
		v, err = client.Discovery().ServerVersion()

		if err == nil {
			return true, nil
		}

		lastErr = err
		res.DebugLog(fmt.Sprintf("Unexpected error discovering Kubernetes version (attempt %v): %v", retries, err))
		retries++
		return false, nil
	})

	if err != nil {
		return nil, lastErr
	}

	if retries > 0 {
		res.DebugLog(fmt.Sprintf("Initial connection to the Kubernetes API server was retried %d times.", retries))
	}

	res.DebugLog(fmt.Sprintf("Running in Kubernetes cluster version v%v.%v (%v) - git (%v) commit %v - platform %v",
		v.Major, v.Minor, v.GitVersion, v.GitTreeState, v.GitCommit, v.Platform))

	return client, nil

}
