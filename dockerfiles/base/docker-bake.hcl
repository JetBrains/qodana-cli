group "all" {
  targets = [
    "jvm-community", "jvm", "python-community", "python",
    "dotnet-community", "dotnet",
    "cpp-community", "cpp", "cpp-community-bookworm", "cpp-bookworm",
    "go-base-latest", "js-base-latest", "php-base-latest", "rust-base-latest",
    "ruby3x"
  ]
}

# Default group for `docker buildx bake` without args
group "default" {
  targets = ["jvm-community", "jvm", "python-community", "python"]
}

# JVM chain: jvm-community is base for jvm and python-community
group "jvm" {
  targets = ["jvm-community", "jvm", "python-community", "python"]
}

# .NET chain
group "dotnet" {
  targets = ["dotnet-community", "dotnet"]
}

# Clang Trixie (20, 21) - fast, modern LLVM
group "clang" {
  targets = ["cpp", "cpp-community"]
}

# Clang Bookworm - split by version for max parallelism
group "clang-16" {
  targets = ["cpp-community-bookworm-16-latest", "cpp-base-bookworm-16-latest"]
}
group "clang-17" {
  targets = ["cpp-community-bookworm-17-latest", "cpp-base-bookworm-17-latest"]
}
group "clang-18" {
  targets = ["cpp-community-bookworm-18-latest", "cpp-base-bookworm-18-latest"]
}
group "clang-19" {
  targets = ["cpp-community-bookworm-19-latest", "cpp-base-bookworm-19-latest"]
}

# Standalone images - each in own job for parallelism
group "go" {
  targets = ["go-base-latest"]
}
group "js" {
  targets = ["js-base-latest"]
}
group "php" {
  targets = ["php-base-latest"]
}
group "rust" {
  targets = ["rust-base-latest"]
}

# Ruby versions share base, build together
group "ruby" {
  targets = ["ruby3x"]
}

target "jvm-community" {
  tags = [
    "registry.jetbrains.team/p/sa/containers/qodana:jvm-community-base-latest"
  ]
  platforms = ["linux/amd64", "linux/arm64"]
  dockerfile = "jvm-community.Dockerfile"
}

target "jvm" {
  contexts = {
    jvm-community = "target:jvm-community"
  }
  tags = [
    "registry.jetbrains.team/p/sa/containers/qodana:jvm-base-latest"
  ]
  platforms = ["linux/amd64", "linux/arm64"]
  dockerfile = "jvm.Dockerfile"
}

target "python-community" {
  contexts = {
    jvm-community = "target:jvm-community"
  }
  tags = [
    "registry.jetbrains.team/p/sa/containers/qodana:python-community-base-latest"
  ]
  platforms = ["linux/amd64", "linux/arm64"]
  dockerfile = "python-community.Dockerfile"
}

target "python" {
  contexts = {
    python-community = "target:python-community"
  }
  tags = [
    "registry.jetbrains.team/p/sa/containers/qodana:python-base-latest"
  ]
  platforms = ["linux/amd64", "linux/arm64"]
  dockerfile = "python.Dockerfile"
}

target "go-base-latest" {
  tags = ["registry.jetbrains.team/p/sa/containers/qodana:go-base-latest"]
  platforms = ["linux/amd64", "linux/arm64"]
  dockerfile = "go.Dockerfile"
}

target "js-base-latest" {
  tags = ["registry.jetbrains.team/p/sa/containers/qodana:js-base-latest"]
  platforms = ["linux/amd64", "linux/arm64"]
  dockerfile = "js.Dockerfile"
}

target "php-base-latest" {
  tags = ["registry.jetbrains.team/p/sa/containers/qodana:php-base-latest"]
  platforms = ["linux/amd64", "linux/arm64"]
  dockerfile = "php.Dockerfile"
}

target "rust-base-latest" {
  tags = ["registry.jetbrains.team/p/sa/containers/qodana:rust-base-latest"]
  platforms = ["linux/amd64", "linux/arm64"]
  dockerfile = "rust.Dockerfile"
}

target "cpp-community" {
  matrix = {
    clang = ["20", "21"]
  }
  name = "cpp-community-${clang}-latest"
  tags = [
    "registry.jetbrains.team/p/sa/containers/qodana:cpp-community-base-${clang}-latest"
  ]
  platforms = ["linux/amd64", "linux/arm64"]
  dockerfile = "cpp-community.Dockerfile"
  args = {
    CLANG = clang
  }
}

target "cpp" {
  contexts = {
    cpp-community = "target:cpp-community-${clang}-latest"
  }
  matrix = {
    clang = ["20", "21"]
  }
  name = "cpp-base-${clang}-latest"
  tags = [
    "registry.jetbrains.team/p/sa/containers/qodana:cpp-base-${clang}-latest"
  ]
  platforms = ["linux/amd64", "linux/arm64"]
  dockerfile = "cpp.Dockerfile"
  args = {
    CLANG = clang
  }
}

# Bookworm-based cpp for clang 16-19 (LLVM apt doesn't have these for Trixie)
target "cpp-community-bookworm" {
  matrix = {
    clang = ["16", "17", "18", "19"]
  }
  name = "cpp-community-bookworm-${clang}-latest"
  tags = [
    "registry.jetbrains.team/p/sa/containers/qodana:cpp-community-base-${clang}-latest"
  ]
  platforms = ["linux/amd64", "linux/arm64"]
  dockerfile = "cpp-community-bookworm.Dockerfile"
  args = {
    CLANG = clang
  }
}

target "cpp-bookworm" {
  contexts = {
    cpp-community = "target:cpp-community-bookworm-${clang}-latest"
  }
  matrix = {
    clang = ["16", "17", "18", "19"]
  }
  name = "cpp-base-bookworm-${clang}-latest"
  tags = [
    "registry.jetbrains.team/p/sa/containers/qodana:cpp-base-${clang}-latest"
  ]
  platforms = ["linux/amd64", "linux/arm64"]
  dockerfile = "cpp.Dockerfile"
  args = {
    CLANG = clang
  }
}

target "dotnet-community" {
  tags = [
    "registry.jetbrains.team/p/sa/containers/qodana:dotnet-community-base-latest"
  ]
  platforms = ["linux/amd64", "linux/arm64"]
  dockerfile = "dotnet-community.Dockerfile"
}

target "dotnet" {
  contexts = {
    dotnet-community = "target:dotnet-community"
  }
  tags = [
    "registry.jetbrains.team/p/sa/containers/qodana:dotnet-base-latest"
  ]
  platforms = ["linux/amd64", "linux/arm64"]
  dockerfile = "dotnet.Dockerfile"
}

target "ruby3x" {
  matrix = {
    version = ["2", "3", "4"]
  }
  name = "ruby-base-3${version}"
  tags = [
    "registry.jetbrains.team/p/sa/containers/qodana:ruby-base-3.${version}-latest"
  ]
  platforms = ["linux/amd64", "linux/arm64"]
  dockerfile = "ruby.Dockerfile"
  args = {
    RUBY_TAG = "3.${version}-debian13-dev"
  }
}