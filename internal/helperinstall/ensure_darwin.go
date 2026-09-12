//go:build darwin

package helperinstall

import "context"

func installCurrentHelper(
	ctx context.Context,
	source, sourceSHA256, token string,
	uid int,
	home, singBox string,
) error {
	return ElevateInstall(ctx, source, sourceSHA256, token, uid, home, singBox)
}
