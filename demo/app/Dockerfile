FROM golang:1.23-alpine AS build
WORKDIR /src
COPY main.go .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/demo main.go

FROM scratch
ARG GIT_SHA
ARG IMAGE_TAG
ENV APP_GIT_SHA=$GIT_SHA
ENV APP_IMAGE_TAG=$IMAGE_TAG
COPY --from=build /out/demo /demo
EXPOSE 8080
ENTRYPOINT ["/demo"]
