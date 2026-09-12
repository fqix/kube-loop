package forwardruntime

import "github.com/fqix/kube-loop/internal/protocol/trojanws"

func trojanPassword(sessionID string, generation uint64) (string, error) {
	return trojanws.DeriveSessionPassword(sessionID, generation)
}
