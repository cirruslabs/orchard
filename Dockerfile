FROM golang:latest AS builder

# Install GoReleaser Pro
RUN echo 'deb [trusted=yes] https://repo.goreleaser.com/apt/ /' | tee /etc/apt/sources.list.d/goreleaser.list
RUN apt update && apt -y install goreleaser-pro

WORKDIR /tmp/orchard
ADD . /tmp/orchard/

RUN goreleaser build --single-target --snapshot --timeout 60m

FROM gcr.io/distroless/base

LABEL org.opencontainers.image.source=https://github.com/cirruslabs/orchard
ENV GIN_MODE=release
ENV ORCHARD_HOME=/data
EXPOSE 6120

COPY --from=builder /tmp/orchard/dist/linux_*/orchard_linux_*/orchard /bin/orchard

ENTRYPOINT ["/bin/orchard"]

# default arguments to run controller
CMD ["controller", "run"]
