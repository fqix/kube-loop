cask "kubeloop-desktop" do
  arch arm: "arm64", intel: "amd64"

  version "3.0.0"
  sha256 arm:   "d5dfc4445b982ea2054f17047f369b74608981c9d49313b3b4584766e1f5c6e3",
         intel: "7989f5cdf5f89866cd0c5f7f3238a43d1bcf78625237261ecea447235544b11f"

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
    "~/Library/Preferences/dev.fengqi.kube-loop.plist",
    "~/Library/Saved Application State/dev.fengqi.kube-loop.savedState",
  ]

  caveats <<~EOS
    On first use, approve the virtual network helper when prompted.
    You can uninstall it later from Settings.
  EOS
end
