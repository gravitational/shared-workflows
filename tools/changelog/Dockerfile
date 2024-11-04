FROM gcr.io/distroless/static-debian12

ARG TARGETARCH
ARG TARGETOS
ARG FILE_NAME="changelog"
ARG SOURCE_FILE_PATH="build/${TARGETOS}/${TARGETARCH}/${FILE_NAME}"
COPY "${SOURCE_FILE_PATH}" "/bin/tool"

ENTRYPOINT [ "/bin/tool" ]
