package daemonset

import (
	"encoding/json"
	"kubetool/src/res"
	"kubetool/src/res/api"
	"kubetool/src/res/deployment"
)

type DaemonSetReadyProbe struct {
	deployment.DeploymentReadyProbe
}

func (this DaemonSetReadyProbe) IsReady(config res.KubeReadyConfiguration) (ok bool, err error) {
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
	cmd = append(cmd, "ds")
	cmd = append(cmd, "-n")
	cmd = append(cmd, config.NameSpace)
	cmd = append(cmd, config.ResourceName)
	cmd = append(cmd, "-ojson")

	out, _, err := res.RunAndOutput(cmd[0], cmd[1:]...)
	if err != nil {
		return
	}

	var ds api.DaemonSet
	err = json.Unmarshal([]byte(out), &ds)
	if err != nil {
		return
	}

	if ds.Status.DesiredNumberScheduled == ds.Status.NumberReady {
		ok = true
		return
	}

	pods, err := this.GetPods(ds.Metadata.Namespace, ds.Kind, ds.Metadata.Name)
	if err != nil {
		return
	}
	res.DebugLog("de:r", ds.Status.DesiredNumberScheduled, ds)

	err = this.CheckPodError(pods)
	return
}
