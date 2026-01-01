FROM golang:1.25.4 AS build

WORKDIR /action

COPY . .

RUN CGO_ENABLED=0 go build -o publish-directory

FROM alpine:latest

COPY --from=build /action/publish-directory /publish-directory

ENTRYPOINT ["/publish-directory"]
