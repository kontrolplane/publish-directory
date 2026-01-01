FROM golang:1.25.4 AS build

WORKDIR /action

COPY . .

RUN CGO_ENABLED=0 go build -o publish-directory

FROM alpine:latest

RUN apk --no-cache add git

COPY --from=build /action/publish-directory /usr/local/bin/publish-dir

ENTRYPOINT ["/usr/local/bin/publish-dir"]
