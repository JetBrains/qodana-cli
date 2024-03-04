group "all" {
  targets = ["debian", "debian-js", "python", "python-js", "dotnet", "go", "js", "php", "rust", "ruby", "cpp", "cdnet"]
}

group "default" {
  targets = ["debian", "debian-js", "python", "python-js"]
}

group "more" {
    targets = ["dotnet", "go", "js", "php"]
}

variable "NODE_TAG" {
  default = "16-bullseye-slim"
}

variable "BASE_TAG" {
  default = "bullseye-slim"
}


target "debian" {
  tags = [
      "registry.jetbrains.team/p/sa/containers/qodana:debian-base-232"
  ]
  args {
    BASE_TAG = "${BASE_TAG}"
  }
  platforms = ["linux/amd64", "linux/arm64"]
  dockerfile = "debian.Dockerfile"
}

target "debian-js" {
  contexts = {
    debianbase = "target:debian"
  }
  args = {
    NODE_TAG = "${NODE_TAG}"
  }
  tags = [
    "registry.jetbrains.team/p/sa/containers/qodana:debian-js-base-232"
  ]
  platforms = ["linux/amd64", "linux/arm64"]
  dockerfile = "debian.js.Dockerfile"
}

target "python" {
  contexts = {
    debianbase = "target:debian"
  }
  tags = [
      "registry.jetbrains.team/p/sa/containers/qodana:python-base-232"
  ]
  platforms = ["linux/amd64", "linux/arm64"]
  dockerfile = "python.Dockerfile"
}

target "python-js" {
  contexts = {
    pythonbase = "target:python"
  }
  args = {
    NODE_TAG = "${NODE_TAG}"
  }
  tags = [
    "registry.jetbrains.team/p/sa/containers/qodana:python-js-base-232"
  ]
  platforms = ["linux/amd64", "linux/arm64"]
  dockerfile = "python.js.Dockerfile"
}

target "dotnet" {
  args = {
    DOTNET_TAG = "6.0-bullseye-slim"
    NODE_TAG = "${NODE_TAG}"
  }
  tags = [
    "registry.jetbrains.team/p/sa/containers/qodana:dotnet-base-232"
  ]
  platforms = ["linux/amd64", "linux/arm64"]
  dockerfile = "dotnet.Dockerfile"
}

target "go" {
  args = {
    GO_TAG = "1.21-bullseye"
    NODE_TAG = "${NODE_TAG}"
  }
  tags = [
    "registry.jetbrains.team/p/sa/containers/qodana:go-base-232"
  ]
  platforms = ["linux/amd64", "linux/arm64"]
  dockerfile = "go.Dockerfile"
}

target "js" {
  args = {
    NODE_TAG = "${NODE_TAG}"
  }
  tags = [
    "registry.jetbrains.team/p/sa/containers/qodana:js-base-232"
  ]
  platforms = ["linux/amd64", "linux/arm64"]
  dockerfile = "js.Dockerfile"
}

target "php" {
  args = {
    PHP_TAG = "8.2-cli-bullseye"
    NODE_TAG = "${NODE_TAG}"
    COMPOSER_TAG="2.6.3"
  }
  tags = [
    "registry.jetbrains.team/p/sa/containers/qodana:php-base-232"
  ]
  platforms = ["linux/amd64", "linux/arm64"]
  dockerfile = "php.Dockerfile"
}

target "rust" {
  args = {
    RUST_TAG = "1.71-slim-bullseye"
    NODE_TAG = "${NODE_TAG}"
  }
  tags = [
    "registry.jetbrains.team/p/sa/containers/qodana:rust-base-232"
  ]
  platforms = ["linux/amd64", "linux/arm64"]
  dockerfile = "rust.Dockerfile"
}

target "ruby" {
  args = {
    RUBY_TAG = "3.0-bullseye"
    NODE_TAG = "${NODE_TAG}"
  }
  tags = [
    "registry.jetbrains.team/p/sa/containers/qodana:ruby-base-232"
  ]
  platforms = ["linux/amd64", "linux/arm64"]
  dockerfile = "ruby.Dockerfile"
}

target "cpp" {
  tags = [
    "registry.jetbrains.team/p/sa/containers/qodana:cpp-base-232"
  ]
  args {
    BASE_TAG = "${BASE_TAG}"
  }
  platforms = ["linux/amd64", "linux/arm64"]
  dockerfile = "cpp.Dockerfile"
}

target "cdnet" {
  tags = [
    "registry.jetbrains.team/p/sa/containers/qodana:cdnet-base-232"
  ]
  args {
    BASE_TAG = "${BASE_TAG}"
  }
  platforms = ["linux/amd64", "linux/arm64"]
  dockerfile = "dotnet.community.Dockerfile"
}
