FROM mcr.microsoft.com/vscode/devcontainers/go:1.25

RUN go install gotest.tools/gotestsum@v1.13.0 && \
    chown -R "vscode:golang" /go/pkg    # Fix ownership
