FROM golang:alpine AS builder
RUN apk add --no-cache gcc libc-dev
WORKDIR /workspace
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a

FROM alpine AS final
ARG ENV
WORKDIR /
COPY --from=builder /workspace/gtemplater .
CMD [ "./gtemplater" ]