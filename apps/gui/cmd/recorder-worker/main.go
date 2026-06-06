package main

import (
	"os"

	"github.com/user/twitcasting-recorder/apps/gui/pkg/workerproc"
)

func main() {
	os.Exit(workerproc.RunWorkerCLI(os.Args[1:]))
}
