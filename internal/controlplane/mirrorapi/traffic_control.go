package mirrorapi

import (
	"github.com/fqix/kube-loop/internal/controlplane/servicebinding"
	"github.com/fqix/kube-loop/internal/controlplane/trafficapi"
	"github.com/fqix/kube-loop/internal/protocol/servicemodel"
	"github.com/fqix/kube-loop/internal/protocol/trafficcontrol"
)

func interceptPorts(
	expected []servicemodel.Port,
	listeners []trafficcontrol.ListenerPort,
) ([]servicebinding.InterceptPort, error) {
	return trafficapi.InterceptPorts(task.Name, expected, listeners)
}
