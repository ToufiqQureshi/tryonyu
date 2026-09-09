FROM golang:1.22-alpine AS build
WORKDIR /app
COPY . .
RUN go build -o /tryon-api .

FROM alpine:3.19
COPY --from=build /tryon-api /tryon-api
EXPOSE 8080
ENTRYPOINT ["/tryon-api"]
