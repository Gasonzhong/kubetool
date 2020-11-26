package main

import (
	"fmt"
	"kubetool/src/res"
	"kubetool/src/res/daemonset"
	"kubetool/src/res/deployment"
	"kubetool/src/res/statefulset"
	"os"
	"strings"
)

func main() {
	fmt.Println("KubeReady 0.2.20200915-cg")
	// config := res.ParseFlag()
	//fmt.Println(res.IsDeloySuccess(3, 3, 50))
	configs := res.ParseFlags()
	for _, cfg := range configs {
		if len(strings.TrimSpace(cfg.ResourceName)) == 0 {
			continue
		}
		fmt.Printf("---\n#execute:\n%s\n", cfg.String())
		probeReady(cfg)
	}

}

func probeReady(config res.KubeReadyConfiguration) {
	res.DebugMode = config.Debug
	var probe res.ReadyProbe
	var rt = strings.ToLower(config.ResourceType)
	if rt == "deployment" {
		probe = deployment.NewClientDeploymentReadyProbe()
	}
	if rt == "statefulset" {
		probe = statefulset.NewClientStatefulSetProbe()
	}
	if rt == "daemonset" {
		probe = daemonset.NewClientDaemonsetProbe()
	}

	ready := false
	var err error = nil
	ready, err = probe.IsReady(config)

	if ready {
		fmt.Println("status: ok")
		return
	}
	var exitCode int
	if err != nil {
		fmt.Println(err)
		exitCode = 1
	} else {
		fmt.Println("timeout")
		exitCode = 2
	}
	fmt.Println(probe.Diagnosis(config))

	os.Exit(exitCode)
}
