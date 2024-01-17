group "default" {
  targets = ["debian", "debian-js", "python", "python-js", "other"]
}

target "debian" {
  tags = [
      "registry.jetbrains.team/p/sa/containers/qodana:debian-base-233"
  ]
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
  tags = [
    "registry.jetbrains.team/p/sa/containers/qodana:debian-js-base-233"
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
      "registry.jetbrains.team/p/sa/containers/qodana:python-base-233"
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
  tags = [
    "registry.jetbrains.team/p/sa/containers/qodana:python-js-base-233"
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

target "other" {
  name = "${edition}-base-233"
  matrix = {
    edition = ["dotnet", "go", "js", "php", "rust", "cpp", "cdnet"]
  }
  tags = [
    "registry.jetbrains.team/p/sa/containers/qodana:${edition}-base-233"
  ]
  platforms = ["linux/amd64", "linux/arm64"]
  dockerfile = "${edition}.Dockerfile"
  cache-from = [
    "type=local,src=docker_cache/other",
  ]
  cache-to = [
    "type=local,dest=docker_cache/other,mode=max",
  ]
}