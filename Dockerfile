FROM golang:latest as go

RUN mkdir /build

COPY aws-env.go /build

WORKDIR /build

RUN go get -u github.com/aws/aws-sdk-go

RUN CGO_ENABLED=0 GOOS=linux go build -v -o awsenv .


FROM scratch

COPY --from=go /build/awsenv /awsenv

COPY --from=go /etc/ssl/certs /etc/ssl/certs

VOLUME /ssm

ENTRYPOINT ["/awsenv"]
