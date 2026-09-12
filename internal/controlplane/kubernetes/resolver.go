package kubernetes

import (
	"github.com/fqix/kube-loop/internal/controlplane/authorization"
	"github.com/fqix/kube-loop/internal/controlplane/controlplaneapi"
)

func subjectFor(identity controlplaneapi.Identity) authorization.Subject {
	return authorization.Subject{
		ID: identity.Subject, Provider: identity.Provider, Groups: append([]string(nil), identity.Groups...),
	}
}
