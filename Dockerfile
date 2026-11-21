# Una sola imagen: el front ya compilado y el binario de Go sobre distroless.
# Las etapas de compilación corren en la plataforma de quien construye y el
# binario se compila cruzado, así la imagen arm64 sale sin QEMU. La etapa
# final no tiene RUN por lo mismo.

FROM --platform=$BUILDPLATFORM public.ecr.aws/docker/library/node:24-alpine AS web
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM public.ecr.aws/docker/library/golang:1.26-alpine AS api
ARG TARGETOS TARGETARCH
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags "-s -w" -o /out/server ./cmd/server \
    && mkdir /out/data

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=api /out/server /app/server
COPY --from=web /src/frontend/dist /app/web
# Carpeta escribible para STORAGE_DRIVER=local (MEDIA_DIR por defecto es ./data/media).
COPY --from=api --chown=nonroot:nonroot /out/data /app/data
ENV PORT=8080 WEB_DIR=/app/web
EXPOSE 8080
ENTRYPOINT ["/app/server"]
