group "all" {
  targets = ["debian", "debian-js", "python", "python-js", "other"]
}

group "default" {
  targets = ["debian", "debian-js", "python", "python-js"]
}

group "more" {
  targets = ["other"]
}

group "clang" {
  targets = ["cpp"]
}

group "ruby" {
  targets = ["ruby3x"]
}

target "debian" {
  tags = [
    "registry.jetbrains.team/p/sa/containers/qodana:debian-base-251"
  ]
  platforms = ["linux/amd64", "linux/arm64"]
  dockerfile = "debian.Dockerfile"
}

target "debian-js" {
  contexts = {
    debianbase = "target:debian"
  }
  tags = [
    "registry.jetbrains.team/p/sa/containers/qodana:debian-js-base-251"
  ]
  platforms = ["linux/amd64", "linux/arm64"]
  dockerfile = "debian.js.Dockerfile"
}

target "python" {
  contexts = {
    debianbase = "target:debian"
  }
  tags = [
    "registry.jetbrains.team/p/sa/containers/qodana:python-base-251"
  ]
  platforms = ["linux/amd64", "linux/arm64"]
  dockerfile = "python.Dockerfile"
}

target "python-js" {
  contexts = {
    pythonbase = "target:python"
  }
  tags = [
    "registry.jetbrains.team/p/sa/containers/qodana:python-js-base-251"
  ]
  platforms = ["linux/amd64", "linux/arm64"]
  dockerfile = "python.js.Dockerfile"
}

target "other" {
  name = "${edition}-base-251"
  matrix = {
    edition = ["dotnet", "go", "js", "php", "rust", "ruby", "cdnet", "cnova"]
  }
  tags = [
    "registry.jetbrains.team/p/sa/containers/qodana:${edition}-base-251"
  ]
  platforms = ["linux/amd64", "linux/arm64"]
  dockerfile = "${edition}.Dockerfile"
}

target "cpp" {
  matrix = {
    clang = ["15", "16", "17", "18"]
  }
  name = "cpp-base-${clang}-251"
  tags = [
    "registry.jetbrains.team/p/sa/containers/qodana:cpp-base-${clang}-251"
  ]
  platforms = ["linux/amd64", "linux/arm64"]
  dockerfile = "cpp.Dockerfile"
  args = {
    CLANG = clang
  }
}

target "ruby3x" {
  matrix = {
    version = ["1", "2", "3", "4"]
  }
  name = "ruby-base-3${version}-251"
  tags = [
    "registry.jetbrains.team/p/sa/containers/qodana:ruby-base-3.${version}-251"
  ]
  platforms = ["linux/amd64", "linux/arm64"]
  dockerfile = "ruby.Dockerfile"
  args = {
    RUBY_TAG = "3.${version}-slim-bookworm"
  }
}