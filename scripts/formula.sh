#!/bin/sh
# Prints the Homebrew formula for a release from its checksums.txt.
# Usage: scripts/formula.sh v0.2.0 checksums.txt
set -eu
tag=$1 sums=$2
sha() { awk -v f="spbstu-mcp_${tag}_$1.tar.gz" '$2 == f { print $1 }' "$sums"; }
url() { echo "https://github.com/kad-xyz/spbstu-mcp/releases/download/${tag}/spbstu-mcp_${tag}_$1.tar.gz"; }
cat <<RUBY
class SpbstuMcp < Formula
  desc "MCP server for SPbPU students: dl.spbstu.ru, university mail, openedu.ru, timetable"
  homepage "https://github.com/kad-xyz/spbstu-mcp"
  license "MIT"

  on_macos do
    on_arm do
      url "$(url darwin_arm64)"
      sha256 "$(sha darwin_arm64)"
    end
    on_intel do
      url "$(url darwin_amd64)"
      sha256 "$(sha darwin_amd64)"
    end
  end

  on_linux do
    on_arm do
      url "$(url linux_arm64)"
      sha256 "$(sha linux_arm64)"
    end
    on_intel do
      url "$(url linux_amd64)"
      sha256 "$(sha linux_amd64)"
    end
  end

  def install
    bin.install "spbstu-mcp"
  end

  def caveats
    <<~EOS
      Store your university login first, then connect the server to Claude:
        security add-generic-password -s spbstu -a LOGIN@edu.spbstu.ru -T /usr/bin/security -w
        claude mcp add spbstu -s user -- #{opt_bin}/spbstu-mcp
      See https://github.com/kad-xyz/spbstu-mcp#quick-start
    EOS
  end

  test do
    assert_match "keychain-service", shell_output("#{bin}/spbstu-mcp -h 2>&1")
  end
end
RUBY
