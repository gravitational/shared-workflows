FROM gcr.io/distroless/static-debian12:nonroot

ARG TARGETARCH
ARG TARGETOS
ARG TOOL_NAME
ARG SOURCE_FILE_PATH="build/${TARGETOS}/${TARGETARCH}/${TOOL_NAME}"
COPY "${SOURCE_FILE_PATH}" "/usr/local/bin/tool"

ENTRYPOINT [ "/usr/local/bin/tool" ]
