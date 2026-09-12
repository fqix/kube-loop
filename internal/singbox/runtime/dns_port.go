//go:build !windows

package runtime

import "github.com/fqix/kube-loop/internal/utils"

func selectDNSPort() (int, error) {
	return utils.FreeTCPPort()
}
