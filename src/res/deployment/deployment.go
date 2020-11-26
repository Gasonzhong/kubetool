package deployment

import (
	"encoding/json"
	"errors"
	"kubetool/src/res"
	"kubetool/src/res/api"
	"strconv"
	"time"
	"utill"
)

type DeploymentReadyProbe struct {
	Config res.KubeReadyConfiguration
}

func (this DeploymentReadyProbe) IsReady(config res.KubeReadyConfiguration) (ok bool, err error) {
	this.Config = config
	err = nil
	ok = false
	deploymentName := config.ResourceName

	deploymentNameSpace := config.NameSpace
	cmd := make([]string, 0, 0)
	cmd = append(cmd, config.KubectlPath)

	if len(config.KubeConfigPath) > 0 {
		cmd = append(cmd, "--kubeconfig")
		cmd = append(cmd, config.KubeConfigPath)
	}
	cmd = append(cmd, "get")
	cmd = append(cmd, "deployment")
	cmd = append(cmd, "-n")
	cmd = append(cmd, deploymentNameSpace)
	cmd = append(cmd, deploymentName)
	cmd = append(cmd, "-ojson")
	cmd = append(cmd, "-w")
	res.DebugLog("cmd::", cmd)
	process, std, _, err := res.RunAndPipe(cmd[0], cmd[1:]...)
	defer res.KillAndWaitProcess(process)

	go res.WatchProcessOverTime(process, time.Second*time.Duration(config.IntervalSecond))

	wjr := res.NewWaveJsonReader(std)
	for {
		var strs []string
		strs, err = wjr.Read()
		if err != nil {
			res.DebugLog(err)
			return false, nil
		}

		if len(strs) > 0 {
			for _, str := range strs {
				deployData := api.Deployment{}
				json.Unmarshal([]byte(str), &deployData)
				if deployData.Status.ReadyReplicas == deployData.Status.Replicas {
					return true, nil
				}
				ok, err = this.CheckReplicasets(config)
				if err != nil {
					return
				}
			}
		}
	}
	return

}
func (this DeploymentReadyProbe) Diagnosis(config res.KubeReadyConfiguration) string {
	return ""
}
func (this DeploymentReadyProbe) CheckReplicasets(config res.KubeReadyConfiguration) (ok bool, err error) {
	this.Config = config
	err = nil
	ok = false
	deploymentName := config.ResourceName
	deploymentKind := "Deployment"
	deploymentNameSpace := config.NameSpace
	cmd := make([]string, 0, 0)
	cmd = append(cmd, config.KubectlPath)

	if len(config.KubeConfigPath) > 0 {
		cmd = append(cmd, "--kubeconfig")
		cmd = append(cmd, config.KubeConfigPath)
	}
	cmd = append(cmd, "get")
	cmd = append(cmd, "rs")
	cmd = append(cmd, "-n")
	cmd = append(cmd, deploymentNameSpace)
	cmd = append(cmd, "-ojson")
	res.DebugLog("cmd::", cmd)

	out, _, err := res.RunAndOutput(cmd[0], cmd[1:]...)
	if err != nil {
		return
	}
	// res.DebugLog(out)
	var rss []api.ReplicaSet
	var listObject api.List
	var currentRs *api.ReplicaSet = nil
	json.Unmarshal([]byte(out), &listObject)
	listData, _ := json.Marshal(listObject.Items)
	json.Unmarshal(listData, &rss)
	// res.DebugLog(rss)
	for rsIdx, rs := range rss {
		for _, ref := range rs.Metadata.OwnerReferences {
			if ref.Kind == deploymentKind && ref.Name == deploymentName {
				// utill.Stde("loop for:", rs)
				if currentRs == nil {
					// utill.Stde("set null")
					currentRs = &rss[rsIdx]
					continue
				}
				// utill.Stde("lv is ", *currentRs)
				lv, _ := strconv.Atoi(currentRs.Metadata.Annotations["deployment.kubernetes.io/revision"])
				cv, _ := strconv.Atoi(rs.Metadata.Annotations["deployment.kubernetes.io/revision"])
				// utill.Stde("lv:", lv, "cv:", cv)
				if cv > lv {
					currentRs = &rss[rsIdx]
				}

			}
		}
	}
	res.DebugLog("Rs is ", utill.ToJSON(currentRs))

	// utill.Stde(utill.ToJSON(currentRs))
	if currentRs == nil {
		err = errors.New("ReplicaSet's Deployment reference not found :" + deploymentNameSpace + "/" + deploymentName)
		return
	}
	if currentRs.Status.Replicas > 0 && currentRs.Status.Replicas == currentRs.Status.ReadyReplicas {
		//running ready
		ok = true
		return
	}
	//replicaset fail check
	cds := currentRs.Status.Conditions
	err = this.CheckReplicaFailed(cds)
	if err != nil {
		return
	}

	//pod fail probe
	pods, err := this.GetPods(deploymentNameSpace, "ReplicaSet", currentRs.Metadata.Name)
	if err != nil {
		return
	}
	err = this.CheckPodError(pods)

	return
}

func (this DeploymentReadyProbe) CheckReplicaFailed(cds []api.ReplicatSetStatusCondition) (err error) {
	for _, item := range cds {
		if item.Type == "ReplicaFailure" && item.Status == "True" && item.Reason == "FailedCreate" {
			err = errors.New(item.Message)
			return
		}
	}
	return nil
}

func (this DeploymentReadyProbe) CheckPodError(pods []api.Pod) (err error) {
	res.DebugLog("config::", this.Config)
	err = nil
	for _, po := range pods {
		if po.Status.Phase == "Failed" || po.Status.Phase == "Unknown" {
			err = errors.New("Pod " + po.Metadata.Name + " status is " + po.Status.Phase)
			return
		}
		res.DebugLog("mark 1", po)
		for _, cs := range po.Status.ContainerStatuses {
			wa, wok := cs.State["waiting"]
			if wok {
				res.DebugLog("mark 2", wa)
				reason, rok := wa["reason"]
				if rok && (reason == "CrashLoopBackOff" || reason == "ImagePullBackOff" || reason == "CreateContainerConfigError" || reason == "ContainerCannotRun") {
					res.DebugLog("mark 3", reason)

					err = errors.New("container " + po.Metadata.Namespace + "/" + po.Metadata.Name + "/" + cs.Name + "  is " + reason)
					return
				}
			}
		}
	}
	return
}
func (this DeploymentReadyProbe) GetPods(namespace string, ownerKind string, rsName string) (ret []api.Pod, err error) {
	ret = make([]api.Pod, 0, 0)
	cmd := make([]string, 0, 0)
	cmd = append(cmd, this.Config.KubectlPath)
	if len(this.Config.KubeConfigPath) > 0 {
		cmd = append(cmd, "--kubeconfig")
		cmd = append(cmd, this.Config.KubeConfigPath)
	}
	cmd = append(cmd, "get")
	cmd = append(cmd, "pod")
	cmd = append(cmd, "-n")
	cmd = append(cmd, namespace)
	cmd = append(cmd, "-ojson")
	res.DebugLog("cmd::", cmd)

	out, _, err := res.RunAndOutput(cmd[0], cmd[1:]...)
	if err != nil {
		return
	}

	// utill.Stde(cmd)

	var pods []api.Pod
	var listObject api.List

	json.Unmarshal([]byte(out), &listObject)
	listData, _ := json.Marshal(listObject.Items)
	json.Unmarshal(listData, &pods)
	// utill.Stde(pods)
	for _, pod := range pods {
		for _, ref := range pod.Metadata.OwnerReferences {
			if ref.Kind == ownerKind && ref.Name == rsName {
				ret = append(ret, pod)
			}
		}
	}
	res.DebugLog(utill.ToJSON(ret))
	return
}
