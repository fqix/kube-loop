class KubeloopTui < Formula
  desc "K9s-style terminal client for KubeLoop"
  homepage "https://fengqi-dev.github.io/kube-loop/"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/fqix/kube-loop/releases/download/v3.0.0/kubeloop-tui-3.0.0-darwin-arm64.tar.gz"
      sha256 "bfbe5a4d693e7416bfdd8b81d74c6a0ab72c382ffd52c60b69e054c04b95f1df"
    else
      url "https://github.com/fqix/kube-loop/releases/download/v3.0.0/kubeloop-tui-3.0.0-darwin-amd64.tar.gz"
      sha256 "670badab7e7e01d847bb29486903c518fad39143faa3ae8fb4f1c2bf46603fa6"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/fqix/kube-loop/releases/download/v3.0.0/kubeloop-tui-3.0.0-linux-arm64.tar.gz"
      sha256 "8208f2f5f671a43bdc5f7596564d45bb3aff01cb284e4649b939fb4af4294a0f"
    else
      url "https://github.com/fqix/kube-loop/releases/download/v3.0.0/kubeloop-tui-3.0.0-linux-amd64.tar.gz"
      sha256 "eabfc57ee75b3b07a348a3996445cdbe48c5098b40efad24a570e9827d2be762"
    end
  end

  def install
    bin.install "kubeloop"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/kubeloop --version")
  end
end
