FROM node:22 AS build
WORKDIR /app
COPY . .
RUN [ ! -f config/config.json ] && cp config/config.json.temp config/config.json
RUN npm --loglevel silly ci --omit=dev


FROM gcr.io/distroless/nodejs22-debian12:nonroot
LABEL org.opencontainers.image.source="https://github.com/flll/receipt-system" \
      org.opencontainers.image.title="ePOS領収書管理システム" \
      org.opencontainers.image.description="https://receipt-view.lll.fish/receipt?uuid=00000000-0000-0000-0000-000000000000"
COPY --from=build /app /app
WORKDIR /app
CMD ["index.js"]
STOPSIGNAL SIGTERM
