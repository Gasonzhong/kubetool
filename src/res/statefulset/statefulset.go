package statefulset

import (
	"encoding/json"
	"kubetool/src/res"
	"kubetool/src/res/api"
	"kubetool/src/res/deployment"
)

type StatefulSetReadyProbe struct {
	deployment.DeploymentReadyProbe
}

func (this StatefulSetReadyProbe) IsReady(config res.KubeReadyConfiguration) (ok bool, err error) {
	this.Config = config
	err = nil
	ok = false

	cmd := make([]string, 0, 0)
	cmd = append(cmd, config.KubectlPath)
	if len(config.KubeConfigPath) > 0 {
		cmd = append(cmd, "--kubeconfig")
		cmd = append(cmd, config.KubeConfigPath)
	}
	cmd = append(cmd, "get")
	cmd = append(cmd, "sts")
	cmd = append(cmd, "-n")
	cmd = append(cmd, config.NameSpace)
	cmd = append(cmd, config.ResourceName)
	cmd = append(cmd, "-ojson")

	out, _, err := res.RunAndOutput(cmd[0], cmd[1:]...)
	if err != nil {
		return
	}

	var sts api.StatefulSet
	err = json.Unmarshal([]byte(out), &sts)
	if err != nil {
		return
	}

	if sts.Status.Replicas == sts.Status.ReadyReplicas {
		ok = true
		return
	}

	pods, err := this.GetPods(sts.Metadata.Namespace, sts.Kind, sts.Metadata.Name)
	if err != nil {
		return
	}

	err = this.CheckPodError(pods)
	return
}
