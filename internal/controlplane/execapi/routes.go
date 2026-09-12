package execapi

import (
	"github.com/fqix/kube-loop/internal/controlplane"
	"github.com/fqix/kube-loop/internal/controlplane/sessionroute"
)

type Routes struct{ *Service }

func NewRoutes(service *Service) *Routes { return &Routes{Service: service} }

func (handler *Routes) Endpoints() controlplane.ExecEndpoints {
	return controlplane.ExecEndpoints{
		Create: sessionroute.WithSession(handler.sessions, handler.create),
		Stream: sessionroute.WithTask(handler.sessions, handler.stream),
	}
}
