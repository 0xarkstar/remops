class Remops < Formula
  desc "Agent-first CLI for remote Docker operations"
  homepage "https://github.com/0xarkstar/remops"
  license "Apache-2.0"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/0xarkstar/remops/releases/download/v#{version}/remops_#{version}_darwin_arm64.tar.gz"
    else
      url "https://github.com/0xarkstar/remops/releases/download/v#{version}/remops_#{version}_darwin_amd64.tar.gz"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/0xarkstar/remops/releases/download/v#{version}/remops_#{version}_linux_arm64.tar.gz"
    else
      url "https://github.com/0xarkstar/remops/releases/download/v#{version}/remops_#{version}_linux_amd64.tar.gz"
    end
  end

  def install
    bin.install "remops"
  end

  test do
    system "#{bin}/remops", "version"
  end
end
