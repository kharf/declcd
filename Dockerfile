FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY bin/controller .
USER 65532:65532
ENTRYPOINT ["/controller"]
