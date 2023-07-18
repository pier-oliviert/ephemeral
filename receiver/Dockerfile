FROM golang:alpine AS builder

WORKDIR /src
# This is to cache the module dependencies in a layer
COPY go.mod go.sum ./
RUN CGOENABLED=0 GOOS=linux GOARCH=amd64 go mod download

COPY / ../operator
# Build can't be optimized, copy the whole thing
COPY . .
RUN CGOENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/receiver ./cmd

FROM alpine:latest
RUN apk --no-cache add ca-certificates
COPY --from=builder /bin/receiver /bin/receiver
EXPOSE 3333

CMD ["/bin/receiver"]
