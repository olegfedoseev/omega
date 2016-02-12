FROM golang:1.5.3-alpine
MAINTAINER Oleg Fedoseev <oleg.fedoseev@me.com>

#RUN apk add --update git

WORKDIR /go/src/github.com/olegfedoseev/omega
COPY . /go/src/github.com/olegfedoseev/omega
RUN go build -o /omega github.com/olegfedoseev/omega

ENV PORT 80
EXPOSE 80
ENTRYPOINT ["/omega"]
