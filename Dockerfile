FROM golang:1.25 AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd
COPY internal/ ./internal
COPY pkg/ ./pkg
RUN CGO_ENABLED=0 go build -o main ./cmd/api

FROM scratch AS final
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /app/main /main
COPY migrations/ /migrations
EXPOSE 8081
EXPOSE 8082
ENTRYPOINT ["/main"]

