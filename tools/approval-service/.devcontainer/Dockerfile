FROM mcr.microsoft.com/vscode/devcontainers/go

RUN go install gotest.tools/gotestsum@latest && \
    chown -R "vscode:golang" /go/pkg    # Fix ownership
