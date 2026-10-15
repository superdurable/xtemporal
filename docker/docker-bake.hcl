variable "IMAGE_NAME" {
  default = "superdurable/xtemporal"
}

variable "IMAGE_SHA_TAG" {
  default = "dev"
}

variable "IMAGE_VERSION_TAG" {
  default = ""
}

variable "IMAGE_MINOR_TAG" {
  default = ""
}

variable "TAG_LATEST" {
  default = false
}

variable "TAG_MAIN" {
  default = false
}

variable "IMAGE_VERSION" {
  default = "unknown"
}

variable "UPSTREAM_VERSION" {
  default = "unknown"
}

variable "REVISION" {
  default = "unknown"
}

variable "ALPINE_TAG" {
  default = "3.24.1"
}

target "server" {
  context = "docker"
  dockerfile = "targets/server.Dockerfile"
  args = {
    ALPINE_TAG = ALPINE_TAG
  }
  tags = compact([
    "${IMAGE_NAME}:${IMAGE_SHA_TAG}",
    IMAGE_VERSION_TAG != "" ? "${IMAGE_NAME}:${IMAGE_VERSION_TAG}" : "",
    IMAGE_MINOR_TAG != "" ? "${IMAGE_NAME}:${IMAGE_MINOR_TAG}" : "",
    TAG_LATEST ? "${IMAGE_NAME}:latest" : "",
    TAG_MAIN ? "${IMAGE_NAME}:main" : "",
  ])
  platforms = ["linux/amd64", "linux/arm64"]
  labels = {
    "org.opencontainers.image.title" = "xtemporal"
    "org.opencontainers.image.description" = "xtemporal server and SQL administration tool"
    "org.opencontainers.image.source" = "https://github.com/superdurable/xtemporal"
    "org.opencontainers.image.url" = "https://github.com/superdurable/xtemporal"
    "org.opencontainers.image.licenses" = "MIT"
    "org.opencontainers.image.version" = IMAGE_VERSION
    "org.opencontainers.image.revision" = REVISION
    "org.opencontainers.image.created" = timestamp()
    "com.superdurable.xtemporal.upstream-version" = UPSTREAM_VERSION
  }
}
