package res

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type KubeReadyConfiguration struct {
	IntervalSecond        int
	ResourceType          string
	ResourceName          string
	NameSpace             string
	PendingTimeout        int64
	ProbeTimeout          int64
	ContainerRestartLimit int64
	KubeConfigPath        string
	KubectlPath           string
	SuccThreshold         int
	Debug                 bool
}

func GetDefaultConfiguration() KubeReadyConfiguration {
	var c KubeReadyConfiguration

	c.IntervalSecond = 10
	c.KubeConfigPath = ""
	c.KubectlPath = ""
	c.Debug = false
	return c
}
func ParseFlag() (c KubeReadyConfiguration, fFile string) {
	c = GetDefaultConfiguration()
	fFile = ""
	flag.IntVar(&c.IntervalSecond, "i", c.IntervalSecond, "Request interval in second")
	flag.StringVar(&c.ResourceType, "t", "Deployment", "Resource type support Deployment StatefulSet and DaemonSet")
	flag.StringVar(&c.ResourceName, "r", "", "Resouce name")
	flag.StringVar(&c.NameSpace, "n", "", "Namespace of Resource")
	flag.Int64Var(&c.PendingTimeout, "pending-to", 1200, "pending timeout seconds")
	flag.Int64Var(&c.ProbeTimeout, "probe-to", 1200, "probe timeout seconds")
	flag.IntVar(&c.SuccThreshold, "succ-threshold", 50, "percent to judge succe 0-100")
	flag.StringVar(&c.KubeConfigPath, "kubeconfig", "", "kube-config path")
	flag.StringVar(&c.KubectlPath, "kubectl-path", "kubectl", "args from file")
	flag.Int64Var(&c.ContainerRestartLimit, "restart-limit", 10, "restart limit")
	flag.StringVar(&fFile, "f", "", "kubectl path")

	flag.BoolVar(&c.Debug, "debug", false, "debug mode more log")

	flag.Parse()
	if c.Debug {
		fmt.Println("args::", strings.Join(os.Args, "|"))
	}

	// if len(c.NameSpace)*len(c.ResourceName) == 0 {
	// 	flag.Usage()
	// 	os.Exit(1)
	// }
	return

}

func ParseFlags() (ret []KubeReadyConfiguration) {
	ret = make([]KubeReadyConfiguration, 0)
	argConfig, fromFile := ParseFlag()

	//from command line
	if len(fromFile) == 0 {
		ret = append(ret, argConfig)
	} else {
		ret = parseConfigsFromFile(fromFile)
	}
	//from file

	return ret
}

func parseConfigsFromFile(path string) (ret []KubeReadyConfiguration) {
	ret = make([]KubeReadyConfiguration, 0)
	// --kubeconfig ${appWorkspace}/kubeconfig -n ${namespace} -t ${resourceType} -r ${resourceName} -to ${timeout}
	fp, err := filepath.Abs(path)
	if err != nil {
		log.Fatal(err)
	}

	_, err = os.Stat(fp)
	if err != nil {
		//		fmt.Println("path:"+path+" is invalid")
		return ret
	}
	var file *os.File
	file, err = os.OpenFile(fp, os.O_RDONLY, os.ModePerm)
	if err != nil {
		fmt.Println("read file " + fp + " err")
		os.Exit(1)
		return
	}
	defer file.Close()
	var fileData []byte
	fileData, err = ioutil.ReadAll(file)
	fileDataString := string(fileData)
	lineSplited := regexp.MustCompile("[\\r\\n]+").Split(fileDataString, -1)
	for _, line := range lineSplited {
		parsed := parseConfigFromLine(line)
		if len(strings.TrimSpace(parsed.ResourceName)) > 0 {
			ret = append(ret, parsed)
		}
	}
	return ret

}

func parseConfigFromLine(lineCmd string) (ret KubeReadyConfiguration) {
	paramMap := parseLineCmdToMap(lineCmd)
	ret = GetDefaultConfiguration()

	ret.IntervalSecond = getIntFromMap(paramMap, "i", ret.IntervalSecond)
	// 类型 Deployment Daementset Statefulset
	ret.ResourceType = getStringFromMap(paramMap, "t", "Deployment")
	ret.ResourceName = getStringFromMap(paramMap, "r", "")
	ret.NameSpace = getStringFromMap(paramMap, "n", "")
	// 等待超时
	ret.PendingTimeout = int64(getIntFromMap(paramMap, "pending-to", 1200))
	// 探针超时
	ret.ProbeTimeout = int64(getIntFromMap(paramMap, "probe-to", 1200))
	ret.ContainerRestartLimit=int64(getIntFromMap(paramMap, "restart-limit", 10))
	// 成功率 SuccThreshold/100 , 如成功率50，deployment中5个pod，成功3个就为成功部署
	ret.SuccThreshold = getIntFromMap(paramMap, "succ-threshold", 50)
	ret.KubeConfigPath = getStringFromMap(paramMap, "kubeconfig", "")
	ret.KubectlPath = getStringFromMap(paramMap, "kubectl-path", "kubectl")
	ret.Debug = getBoolFromMap(paramMap, "debug", false)

	return ret
}
func getIntFromMap(m map[string]string, key string, defaultVal int) int {
	val, ok := m[key]
	if !ok {
		return defaultVal
	}
	ival, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return ival
}
func getStringFromMap(m map[string]string, key string, defaultVal string) string {
	val, ok := m[key]
	if !ok {
		return defaultVal
	}
	if len(val) == 0 {
		return defaultVal
	}
	return val
}
func getBoolFromMap(m map[string]string, key string, defaultVal bool) bool {
	val, ok := m[key]
	if !ok {
		return defaultVal
	}
	if val == "" {
		return true
	}
	if strings.ToLower(val) == "false" {
		return false
	}
	return defaultVal
}

func parseLineCmdToMap(lineCmd string) (ret map[string]string) {
	ret = make(map[string]string)
	blankSplited := regexp.MustCompile("\\s+").Split(lineCmd, -1)
	var paramKey = ""
	var paramVal = ""
	for _, word := range blankSplited {
		if strings.HasPrefix(word, "-") || strings.HasPrefix(word, "--") {
			//new param save old
			ret[paramKey] = paramVal
			paramKey = regexp.MustCompile("^-{1,2}").ReplaceAllString(word, "")
			paramVal = ""
		} else {
			paramVal = word
		}

	}
	ret[paramKey] = paramVal
	return ret
}

func (this KubeReadyConfiguration) String() string {
	debugString := "false"
	if this.Debug {
		debugString = "true"
	}
	return fmt.Sprintf("intervalSecond %d \nresourceType: %s \nresourceName: %s \nnameSpace: %s \nPendingTimeout: %d \nProbeTimeout: %d \nContainerRestartLimit: %d \nkubeConfigPath: %s \nkubectlPath: %s\nsuccThreshold: %d\ndebug: %s",
		this.IntervalSecond, this.ResourceType, this.ResourceName, this.NameSpace, this.PendingTimeout, this.ProbeTimeout,this.ContainerRestartLimit, this.KubeConfigPath, this.KubectlPath, this.SuccThreshold, debugString)

}
