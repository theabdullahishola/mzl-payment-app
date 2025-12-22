FROM golang:1.24-alpine AS builder

RUN apk add --no-cache ca-certificates git

WORKDIR /app
ENV GOPROXY=https://proxy.golang.org,direct

COPY go.mod go.sum ./

COPY prisma ./prisma


COPY . .


RUN rm -rf vendor


RUN go run -mod=mod github.com/steebchen/prisma-client-go generate


RUN CGO_ENABLED=0 GOOS=linux go build -mod=mod -o main ./cmd/server/main.go

FROM alpine:latest
WORKDIR /app
RUN apk --no-cache add ca-certificates
COPY --from=builder /app/main .
EXPOSE 8080
CMD ["./main"]