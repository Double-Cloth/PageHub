FROM golang:1.25.4-alpine AS build
WORKDIR /src
COPY main.go .
RUN CGO_ENABLED=0 go build -trimpath -o /web main.go
FROM alpine:3.22.2
COPY --from=build /web /web
CMD ["/web"]
