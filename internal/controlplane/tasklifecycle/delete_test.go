package tasklifecycle

import (
	"testing"
	"time"

	"github.com/fqix/kube-loop/internal/controlplane/storage"
	"github.com/fqix/kube-loop/internal/protocol/remotetask"
)

func TestDeleteDistinguishesRemovedTaskFromPausedTask(t *testing.T) {
	repository := &competingTaskRepository{task: storage.Task{
		ID: "task", State: remotetask.Stopped,
	}}
	task, err := Delete(t.Context(), repository, "task", time.Now)
	if err != nil || task.State != remotetask.Deleted || repository.updates != 1 {
		t.Fatalf("deleted task=%#v updates=%d err=%v", task, repository.updates, err)
	}
	task, err = Delete(t.Context(), repository, "task", time.Now)
	if err != nil || task.State != remotetask.Deleted || repository.updates != 1 {
		t.Fatalf("repeated delete task=%#v updates=%d err=%v", task, repository.updates, err)
	}
}
