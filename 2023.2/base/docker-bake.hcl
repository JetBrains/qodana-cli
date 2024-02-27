group "default" {
  targets = ["debian", "debian-js", "python", "python-js", "dotnet", "go", "js", "php"]
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
  cache-from = [
    "type=local,src=docker_cache/debian",
  ]
  cache-to = [
    "type=local,dest=docker_cache/debian,mode=max",
  ]
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
  cache-from = [
    "type=local,src=docker_cache/debian_js",
  ]
  cache-to = [
    "type=local,dest=docker_cache/debian_js,mode=max",
  ]
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
  cache-from = [
    "type=local,src=docker_cache/python",
  ]
  cache-to = [
    "type=local,dest=docker_cache/python,mode=max",
  ]
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
  cache-from = [
    "type=local,src=docker_cache/python_js",
  ]
  cache-to = [
    "type=local,dest=docker_cache/python_js,mode=max",
  ]
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
  cache-from = [
    "type=local,src=docker_cache/dotnet",
  ]
  cache-to = [
    "type=local,dest=docker_cache/dotnet,mode=max",
  ]
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
  cache-from = [
    "type=local,src=docker_cache/go",
  ]
  cache-to = [
    "type=local,dest=docker_cache/go,mode=max",
  ]
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
  cache-from = [
    "type=local,src=docker_cache/js",
  ]
  cache-to = [
    "type=local,dest=docker_cache/js,mode=max",
  ]
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
  cache-from = [
    "type=local,src=docker_cache/php",
  ]
  cache-to = [
    "type=local,dest=docker_cache/php,mode=max",
  ]
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
  cache-from = [
    "type=local,src=docker_cache/rust",
  ]
  cache-to = [
    "type=local,dest=docker_cache/rust,mode=max",
  ]
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
  cache-from = [
    "type=local,src=docker_cache/ruby",
  ]
  cache-to = [
    "type=local,dest=docker_cache/ruby,mode=max",
  ]
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
  cache-from = [
    "type=local,src=docker_cache/cpp",
  ]
  cache-to = [
    "type=local,dest=docker_cache/cpp,mode=max",
  ]
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
  cache-from = [
    "type=local,src=docker_cache/cdnet",
  ]
  cache-to = [
    "type=local,dest=docker_cache/cdnet,mode=max",
  ]
}
