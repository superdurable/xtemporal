variable "IMAGE_NAME" {
  default = "superdurable/dex-engine-service"
}

variable "IMAGE_SHA_FULL_TAG" {
  default = "dev"
}

variable "IMAGE_RELEASE_TAG" {
  default = ""
}

variable "TEMPORAL_SHA" {
  default = ""
}

# ALPINE_TAG is the single source of truth for the Alpine base image version.
# NOTE: We use just the tag without a digest pin because digest-pinned manifest lists
# cause platform resolution issues in multi-arch buildx builds (InvalidBaseImagePlatform warnings).
variable "ALPINE_TAG" {
  default = "3.23.5"
}

target "server" {
  context = "docker"
  dockerfile = "targets/server.Dockerfile"
  args = {
    ALPINE_TAG = "${ALPINE_TAG}"
  }
  tags = compact([
    "${IMAGE_NAME}:${IMAGE_SHA_FULL_TAG}",
    IMAGE_RELEASE_TAG != "" ? "${IMAGE_NAME}:${IMAGE_RELEASE_TAG}" : "",
  ])
  platforms = ["linux/amd64", "linux/arm64"]
  labels = {
    "org.opencontainers.image.title" = "Dex Engine Service"
    "org.opencontainers.image.description" = "Dex's customized Temporal Server"
    "org.opencontainers.image.url" = "https://github.com/superdurable/temporal-server"
    "org.opencontainers.image.source" = "https://github.com/superdurable/temporal-server"
    "org.opencontainers.image.licenses" = "MIT"
    "org.opencontainers.image.version" = "${IMAGE_RELEASE_TAG}"
    "org.opencontainers.image.revision" = "${TEMPORAL_SHA}"
    "org.opencontainers.image.created" = timestamp()
    "com.temporal.server.version" = "${IMAGE_RELEASE_TAG}"
  }
}
