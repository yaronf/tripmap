# Cross-compile friendly: build on host arch, emit linux/$TARGETARCH binary.
FROM --platform=$BUILDPLATFORM golang:1.24-bookworm AS build
ARG TARGETOS=linux
ARG TARGETARCH=amd64
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /tripmapd ./cmd/tripmapd

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /tripmapd /tripmapd
USER nonroot:nonroot
EXPOSE 8080
ENV ADDR=:8080
ENTRYPOINT ["/tripmapd"]
