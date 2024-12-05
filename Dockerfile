FROM node:22 AS build-env
WORKDIR /app
COPY . .
RUN [ ! -f config/config.json ] && cp config/config.json.temp config/config.json
RUN npm --loglevel silly ci --omit=dev


FROM gcr.io/distroless/nodejs22-debian12:latest-amd64
COPY --from=build-env /app /app
WORKDIR /app
ENTRYPOINT ["/nodejs/bin/node", "index.js"]
#ENTRYPOINT ["node", "index.js"]
CMD ["view"]
STOPSIGNAL SIGTERM
