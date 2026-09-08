class KubeloopTui < Formula
  desc "K9s-style terminal client for KubeLoop"
  homepage "https://fengqi-dev.github.io/kube-loop/"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/fqix/kube-loop/releases/download/v3.0.1/kubeloop-tui-3.0.1-darwin-arm64.tar.gz"
      sha256 "615221e5bc14308e9f045b59445f755aa10c6fdfa2f8639271bca2112dc3d73c"
    else
      url "https://github.com/fqix/kube-loop/releases/download/v3.0.1/kubeloop-tui-3.0.1-darwin-amd64.tar.gz"
      sha256 "259e423b2d9db0d743f21c8667d9d955574501c2038c47d00b10aa8256a86116"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/fqix/kube-loop/releases/download/v3.0.1/kubeloop-tui-3.0.1-linux-arm64.tar.gz"
      sha256 "491f456de6f1c5df6ee30ab030c1f73b84b28ef9d710069271cee002440b576d"
    else
      url "https://github.com/fqix/kube-loop/releases/download/v3.0.1/kubeloop-tui-3.0.1-linux-amd64.tar.gz"
      sha256 "ae8107a277fb86eed8c040621a7168002123645434614a47bfda6db4678fe572"
    end
  end

  def install
    bin.install "kubeloop"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/kubeloop --version")
  end
end
