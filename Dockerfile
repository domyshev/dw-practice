FROM node:24-alpine AS web
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci
COPY tsconfig.json vite.config.ts ./
COPY web ./web
RUN npm run build

FROM golang:1.25-alpine AS api
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY backend ./backend
RUN CGO_ENABLED=0 go build -trimpath -o /karma ./backend

FROM alpine:3.22
RUN apk add --no-cache ca-certificates && addgroup -g 10001 karma && adduser -D -u 10001 -G karma karma
WORKDIR /app
COPY --from=api /karma /app/karma
COPY --from=web /src/dist /app/dist
RUN mkdir /app/data && chown karma:karma /app/data
USER karma
ENV APP_ENV=production EMAIL_MODE=resend LISTEN_ADDR=0.0.0.0:8080 DATABASE_PATH=/app/data/karma.db
EXPOSE 8080
VOLUME ["/app/data"]
ENTRYPOINT ["/app/karma"]
