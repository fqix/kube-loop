package app

const (
	releaseURL                    = "https://github.com/fengqi-dev/kube-loop/releases"
	authenticationProviderBrowser = "browser"
	serverFileKindFile            = "file"
	serverFileKindDirectory       = "directory"
	fileTransferDirectionDownload = "download"
	remoteStateDisconnected       = "disconnected"
	tunnelModeSOCKS               = "socks"
	tunnelModeTUN                 = "tun"
)

// Event names published to the user interface. They are part of the desktop
// shell contract: the frontend subscribes to these exact strings.
const (
	updateStateEvent             = "update:state"
	serverInventorySnapshotEvent = "server-inventory:snapshot"
	serverFileTransferEvent      = "server-file-transfer:event"
	serverExecEvent              = "server-exec:event"
	dataPlaneStatusEvent         = "dataplane:status"
)
