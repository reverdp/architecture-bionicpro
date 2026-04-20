FROM golang:1.26 AS build

WORKDIR /src
COPY . .

RUN go build -o /out/bionicpro-auth ./cmd/bionicpro-auth

FROM debian:bookworm-slim

WORKDIR /app
COPY --from=build /out/bionicpro-auth /app/bionicpro-auth

EXPOSE 8001

ENTRYPOINT ["/app/bionicpro-auth"]
