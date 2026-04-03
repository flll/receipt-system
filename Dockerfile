FROM golang:1.26.1@sha256:595c7847cff97c9a9e76f015083c481d26078f961c9c8dca3923132f51fe12f1 AS build
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -mod=readonly -ldflags="-s -w" -o receipt-system .

FROM gcr.io/distroless/static-debian13@sha256:47b2d72ff90843eb8a768b5c2f89b40741843b639d065b9b937b07cd59b479c6
LABEL org.opencontainers.image.source="https://github.com/flll/receipt-system" \
      org.opencontainers.image.title="ePOS領収書管理システム" \
      org.opencontainers.image.description="https://receipt-view.lll.fish/receipt?uuid=00000000-0000-0000-0000-000000000000"

COPY --from=build /app/receipt-system /app/receipt-system
COPY --from=build /app/index.html /app/index.html
COPY --from=build /app/views /app/views
COPY --from=build /app/js /app/js
COPY --from=build /app/templates /app/templates
COPY --from=build /app/config /app/config
WORKDIR /app
CMD ["/app/receipt-system"]
STOPSIGNAL SIGTERM
