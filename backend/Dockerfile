FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/api ./cmd/api && CGO_ENABLED=0 go build -o /out/worker ./cmd/worker && CGO_ENABLED=0 go build -o /out/migrate ./cmd/migrate
FROM alpine:3.23
LABEL org.opencontainers.image.source="https://github.com/jcardonne/tullips"
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 app
WORKDIR /app
COPY --from=build /out/ /app/
COPY migrations /app/migrations
USER app
ARG RELEASE_VERSION=dev
ENV RELEASE_VERSION=$RELEASE_VERSION
EXPOSE 8080
CMD ["/app/api"]
