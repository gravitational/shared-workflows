FROM mcr.microsoft.com/vscode/devcontainers/go

RUN apt update && \
    apt install -y --no-install-recommends age

COPY --from=ghcr.io/getsops/sops:v3.9.1 /usr/local/bin/sops /usr/local/bin/sops
