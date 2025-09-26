FROM mcr.microsoft.com/vscode/devcontainers/go:1.25

RUN go install gotest.tools/gotestsum@v1.13.0 && \
    chown -R "vscode:golang" /go/pkg    # Fix ownership

# Attune has not yet added linux/arm64 support
RUN  \
    [ "$(uname -m)" = "x86_64" ] &&  ( \
    curl -fsSL -o /tmp/attune.deb \
    https://github.com/attunehq/attune/releases/download/v0.2.0/attune_v0.2.0_Linux-X64.deb \
    && dpkg -i /tmp/attune.deb \
    && rm /tmp/attune.deb \
    ) || true
