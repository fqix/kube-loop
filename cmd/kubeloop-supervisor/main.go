//go:build darwin

package main

import (
	"os"

	"k8s.io/component-base/cli"

	"github.com/fqix/kube-loop/cmd/kubeloop-supervisor/app"
)

func main() {
	os.Exit(cli.Run(app.NewSupervisorCommand()))
}
