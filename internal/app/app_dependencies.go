package app

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/fqix/kube-loop/internal/client/credentials"
	"github.com/fqix/kube-loop/internal/utils"
)

type appDependencies struct {
	profilePath     string
	credentialStore credentials.Store
	httpClient      *http.Client
}

func appUserLayout(version, profilePath string) (utils.Layout, error) {
	profilePath = strings.TrimSpace(profilePath)
	var layout utils.Layout
	var err error
	if profilePath == "" {
		layout, err = utils.ForVersion(version)
	} else {
		root := filepath.Dir(profilePath)
		if filepath.Base(root) == "config" {
			root = filepath.Dir(root)
		}
		layout, err = utils.New(root)
	}
	if err != nil {
		return utils.Layout{}, err
	}
	if err := layout.Ensure(); err != nil {
		return utils.Layout{}, fmt.Errorf("initialize KubeLoop user directories: %w", err)
	}
	return layout, nil
}
