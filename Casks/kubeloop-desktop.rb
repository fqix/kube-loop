cask "kubeloop-desktop" do
  arch arm: "arm64", intel: "amd64"

  version "3.0.1"
  sha256 arm:   "81e2bcfabbfac8d062280590e6593fd4918b24b15931f52550136cff190104f2",
         intel: "e122221370d04b10857131ac4e9fd59e434662d0949cafa925dbf5d6441a9c13"

  url "https://github.com/fengqi-dev/kube-loop/releases/download/v#{version}/kubeloop-desktop-#{version}-darwin-#{arch}.dmg"
  name "KubeLoop"
  desc "Connect your laptop to Kubernetes like a VPN"
  homepage "https://fengqi-dev.github.io/kube-loop/"

  livecheck do
    url :url
    strategy :github_latest
  end

  depends_on macos: :big_sur

  app "KubeLoop.app"

  # Unsigned / unnotarized builds trip Gatekeeper via com.apple.quarantine.
  # Clear it after install so users do not need a manual xattr.
  postflight_steps do
    run "/usr/bin/xattr",
        args: ["-dr", "com.apple.quarantine", "{{appdir}}/KubeLoop.app"]
  end

  zap trash: [
    "~/.kubeloop",
    "~/Library/Application Support/KubeLoop",
    "~/Library/Caches/KubeLoop",
    "~/Library/Preferences/com.wails.kube-loop.plist",
    "~/Library/Saved Application State/com.wails.kube-loop.savedState",
  ]

  caveats <<~EOS
    On first use, approve the virtual network helper when prompted.
    You can uninstall it later from Settings.
  EOS
end
