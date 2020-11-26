package res

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"os/exec"
	"regexp"
	"time"
	"utill"
)

func RunAndOutput(cmd string, args ...string) (stdout string, stderr string, err error) {
	stdout = ""
	stderr = ""
	err = nil
	process := exec.Command(cmd, args...)
	stdOutPipe, err := process.StdoutPipe()
	if err != nil {
		return
	}
	stdErrPipe, err := process.StderrPipe()
	if err != nil {
		return
	}

	err = process.Start()
	if err != nil {
		return
	}
	stdoutData, err := ioutil.ReadAll(stdOutPipe)
	if err != nil {
		return
	}
	stdout = string(stdoutData)

	stderrData, err := ioutil.ReadAll(stdErrPipe)
	if err != nil {
		return
	}
	stderr = string(stderrData)
	process.Process.Kill()
	process.Wait()

	return
}

func RunAndPipe(cmd string, args ...string) (process *exec.Cmd, stdOutPipe io.ReadCloser, stdErrPipe io.ReadCloser, err error) {

	err = nil
	process = exec.Command(cmd, args...)
	stdOutPipe, err = process.StdoutPipe()
	if err != nil {
		return
	}
	stdErrPipe, err = process.StderrPipe()
	if err != nil {
		return
	}
	err = process.Start()

	return
}

type WaveJsonReader struct {
	data   []byte
	source io.ReadCloser
}

func NewWaveJsonReader(source io.ReadCloser) WaveJsonReader {
	ret := WaveJsonReader{data: make([]byte, 0, 0), source: source}
	return ret
}

func (this *WaveJsonReader) Read() (str []string, err error) {
	str = make([]string, 0)
	tmpData := make([]byte, 1024)
	tmpLen, err := this.source.Read(tmpData)
	// DebugLog("tmpLen", tmpLen)
	if tmpLen > 0 {
		lines := regexp.MustCompile("[\\r\\n]+").Split(string(tmpData[:tmpLen]), -1)
		for _, line := range lines {
			this.data = append(this.data, []byte(line)...)
			if json.Valid(this.data) {
				str = append(str, string(this.data))
				DebugLog("data", "*****", regexp.MustCompile("[\\r\\n\\s]").ReplaceAllString(string(this.data), ""), "****")
				this.data = this.data[0:0]
			}
		}

	} else {
		return str, err
	}

	return
}

func WatchProcessOverTime(process *exec.Cmd, tmout time.Duration) {
	DebugLog("kill process", process.Process.Pid, "after", tmout)
	<-time.After(tmout)
	KillAndWaitProcess(process)
}

func KillAndWaitProcess(process *exec.Cmd) {
	process.Process.Kill()
	process.Wait()
}

func ToJson(in interface{}) string {
	data, err := json.Marshal(in)
	if err != nil {
		return ""
	}
	return string(data)
}

var DebugMode bool = false

func DebugLog(c ...interface{}) {
	if DebugMode {
		utill.Stde(c...)
	}
}
