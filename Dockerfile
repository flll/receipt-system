FROM node:22 AS build-env
WORKDIR /app
COPY . .
COPY config.json.temp config.json
RUN npm --loglevel silly ci --omit=dev


FROM gcr.io/distroless/nodejs22-debian12
COPY --from=build-env /app /app
WORKDIR /app
CMD ["server.js", "view"]
STOPSIGNAL SIGTERM
