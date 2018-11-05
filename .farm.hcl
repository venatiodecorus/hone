secrets = [
    "S3_ACCESS_KEY",
    "S3_SECRET_KEY",
    "DOCKER_USER",
    "DOCKER_PASS"
]

env = [
    "ENGINE=docker",
    "S3_BUCKET=farm-cache-bucket",
    "S3_ENDPOINT=nyc3.digitaloceanspaces.com",
    "S3_ENABLED=true",
    "S3_ACCESS_KEY",
    "S3_SECRET_KEY",
    "DOCKER_USER",
    "DOCKER_PASS",
    "WORKSPACE=dev",
    "VAULT_ADDR=http://127.0.0.1:8200/",
    "VAULT_TOKEN"
]

workspace = "${environ.WORKSPACE}"
engine = "${environ.ENGINE}"

vault {
    address = "${environ.VAULT_ADDR}"
    token = "${environ.VAULT_TOKEN}"
}

kubernetes {
    namespace = "default"
}

cache {
    s3 {
        access_key = "${environ.S3_ACCESS_KEY}"
        secret_key = "${environ.S3_SECRET_KEY}"
        endpoint = "${environ.S3_ENDPOINT}"
        bucket = "${environ.S3_BUCKET}"
        disabled = "${environ.S3_ENABLED != "true"}"
    }
}

job "test" {
    image = "golang:1.11.2"

    inputs = ["./cmd/", "./pkg/", "go.mod", "go.sum"]

    env = {
        "GO111MODULE" = "on"
        "GOCACHE" = "/build/.gocache"
        "GOPATH" = "/build/.go"
    }

    shell = "go test ./cmd/... ./pkg/..."
}

job "build" {
    deps = ["test"]

    image = "golang:1.11.2"

    env = {
        "GO111MODULE" = "on"
        "GOCACHE" = "/build/.gocache"
        "GOPATH" = "/build/.go"
    }

    inputs = ["./cmd/*/*.go", "./pkg/**/*.go", "go.mod", "go.sum"]
    output = "farm"

    shell = "go build -v -o ./farm ./cmd/farm"
}

job "build-mac" {
    deps = ["test"]

    image = "golang:1.11.2"

    env = {
        "GO111MODULE" = "on"
        "GOCACHE" = "/build/.gocachedarwin"
        "GOPATH" = "/build/.go"
        "GOOS" = "darwin"
    }

    inputs = ["./cmd/*/*.go", "./pkg/**/*.go", "go.mod", "go.sum"]
    output = "farm_darwin"

    shell = "go build -v -o ./farm_darwin ./cmd/farm"
}

job "build-kaniko-shim" {
    image = "golang:1.11.2"

    env = {
        "GO111MODULE" = "on"
        "GOCACHE" = "/build/.gocache"
        "GOPATH" = "/build/.go"
    }

    inputs = ["./cmd/*/*.go", "./pkg/**/*.go", "go.mod", "go.sum"]
    output = "docker/kaniko"

    shell = "go build -v -o ./docker/kaniko ./cmd/kaniko"
}

job "build-kaniko-shim-image" {
    image = "justinbarrick/kaniko:latest"

    deps = ["build-kaniko-shim"]
    inputs = ["docker/Dockerfile.kaniko", "docker/kaniko"]

    env = {
        "DOCKER_USER" = "${environ.DOCKER_USER}",
        "DOCKER_PASS" = "${environ.DOCKER_PASS}",
    }

    shell = <<EOF
kaniko --dockerfile=docker/Dockerfile.kaniko --context=/build/docker/ \
    --destination=${environ.DOCKER_USER}/kaniko:latest
EOF

    engine = "kubernetes"
}

job "build-cache-shim" {
    image = "golang:1.11.2"

    env = {
        "GO111MODULE" = "on"
        "GOCACHE" = "/build/.gocache"
        "GOPATH" = "/build/.go"
        "CGO_ENABLED" = "0"
    }

    inputs = ["./cmd/*/*.go", "./pkg/**/*.go", "go.mod", "go.sum"]
    output = "./docker/cache-shim"

    shell = "go build -ldflags '-w -extldflags -static' -o ./docker/cache-shim ./cmd/cache-shim"
}

job "build-cache-shim-image" {
    image = "justinbarrick/kaniko:latest"

    deps = ["build-cache-shim"]
    inputs = ["docker/Dockerfile.cache-shim", "docker/cache-shim"]

    env = {
        "DOCKER_USER" = "${environ.DOCKER_USER}",
        "DOCKER_PASS" = "${environ.DOCKER_PASS}",
    }

    shell = <<EOF
kaniko --dockerfile=docker/Dockerfile.cache-shim --context=/build/docker/ \
    --destination=${environ.DOCKER_USER}/cache-shim:latest
EOF

    engine = "kubernetes"
}

job "images" {
    deps = ["build-cache-shim-image", "build-kaniko-shim-image"]
    image = "alpine"
    shell = "echo images"
}

job "binaries" {
    deps = ["build-cache-shim", "build-kaniko-shim", "build", "build-mac"]
    image = "alpine"
    shell = "echo binaries"
}

job "all" {
    deps = ["images", "binaries"]
    image = "alpine"
    shell = "echo all"
}