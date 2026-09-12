package exchangeapi

import (
	"github.com/fqix/kube-loop/internal/controlplane/controlplaneapi"
	"github.com/fqix/kube-loop/internal/controlplane/trafficapi"
)

func normalizeRequest(spec *Spec) *controlplaneapi.Error {
	if apiError := trafficapi.NormalizeServicePorts(&spec.Service, spec.Ports); apiError != nil {
		return apiError
	}
	return trafficapi.NormalizeLocalTargets(&spec.LocalTargets, spec.Ports)
}

var apiErrors = task.Errors()
