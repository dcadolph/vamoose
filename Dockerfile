# syntax=docker/dockerfile:1

# Build a static, CGO-free vamoose binary.
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-s -w" -o /out/vamoose .

# Minimal runtime image running the Slack server, which also advances each linked
# user's watched holds on its poll loop. Set VAMOOSE_SECRET_KEY so tokens and links
# are encrypted at rest on the volume.
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -u 10001 vamoose
COPY --from=build /out/vamoose /usr/local/bin/vamoose
USER vamoose
ENV VAMOOSE_TIMEZONE=UTC
EXPOSE 8080
ENTRYPOINT ["vamoose"]
CMD ["slack", "--addr", ":8080"]
