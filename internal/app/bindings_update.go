package app

import (
	"context"
	"fmt"
	"time"

	"github.com/fengqi-dev/kube-loop/internal/update"
)

func (a *App) CheckForUpdates() update.Info {
	a.logInfo("checking for application updates")
	checkContext, cancel := context.WithTimeout(a.context(), 20*time.Second)
	defer cancel()
	state := a.checkForUpdates(checkContext)
	a.emit(updateStateEvent, state)
	return state
}

func (a *App) OpenUpdatePage() error {
	a.updateMu.RLock()
	target := a.updateState.URL
	a.updateMu.RUnlock()
	if target == "" {
		target = releaseURL
	}
	a.logInfo("opening application update page")
	if err := a.hostRuntime().OpenURL(target); err != nil {
		a.logError("open update page: " + err.Error())
		return err
	}
	return nil
}

func (a *App) checkForUpdates(ctx context.Context) update.Info {
	a.updateCheck.Lock()
	defer a.updateCheck.Unlock()
	state, err := a.updater.Check(ctx)
	switch {
	case err != nil:
		state.Error = err.Error()
		a.logWarn(fmt.Sprintf("application update check failed: %v", err))
	case state.Available:
		a.logInfo(fmt.Sprintf(
			"application update available: current=%s latest=%s",
			state.CurrentVersion, state.LatestVersion,
		))
	default:
		a.logInfo("application is up to date")
	}
	a.updateMu.Lock()
	a.updateState = state
	a.updateMu.Unlock()
	return state
}
