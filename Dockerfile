# syntax=docker/dockerfile:1

# Build a static, CGO-free vamoose binary. VERSION is stamped into `vamoose version`;
# the default marks a local build not made by the release workflow.
FROM golang:1.26-alpine AS build
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X github.com/dcadolph/vamoose/cmd.version=${VERSION#v}" -o /out/vamoose .

# Minimal runtime image running the Slack server, which also advances each linked
# user's watched holds on its poll loop. Set VAMOOSE_SECRET_KEY so tokens and links
# are encrypted at rest on the volume.
FROM alpine:3.20
LABEL org.opencontainers.image.source="https://github.com/dcadolph/vamoose" \
      org.opencontainers.image.description="Calendar workflow engine for time off, approvals, and quick actions" \
      org.opencontainers.image.licenses="BUSL-1.1"
RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -u 10001 vamoose
COPY --from=build /out/vamoose /usr/local/bin/vamoose
USER vamoose
ENV VAMOOSE_TIMEZONE=UTC
EXPOSE 8080
ENTRYPOINT ["vamoose"]
CMD ["slack", "--addr", ":8080"]
