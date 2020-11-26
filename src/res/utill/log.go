package utill

import (
	"fmt"
	"log"
	"os"
)

var LogLevel = 0

func L(level int, contents ...interface{}) {
	if level >= LogLevel {
		log.Println(contents)
	}
}

func Stde(contents ...interface{}) {
	str := fmt.Sprintln(contents)
	os.Stderr.WriteString(str)
}
