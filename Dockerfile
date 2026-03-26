FROM golang:1.25 AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN [ ! -f config/config.json ] && cp config/config.json.temp config/config.json; true
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o receipt-system .

FROM gcr.io/distroless/static-debian12:nonroot
LABEL org.opencontainers.image.source="https://github.com/flll/receipt-system" \
      org.opencontainers.image.title="ePOS領収書管理システム" \
      org.opencontainers.image.description="https://receipt-view.lll.fish/receipt?uuid=00000000-0000-0000-0000-000000000000"
COPY --from=build /app/receipt-system /app/receipt-system
COPY --from=build /app/index.html /app/index.html
COPY --from=build /app/views /app/views
COPY --from=build /app/js /app/js
COPY --from=build /app/editor /app/editor
COPY --from=build /app/config /app/config
WORKDIR /app
CMD ["/app/receipt-system"]
STOPSIGNAL SIGTERM
