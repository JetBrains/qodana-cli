variable "BASE_REPO" {
  default = "registry.jetbrains.team/p/sa/containers/qodana"
}

# Comma-separated; CI builds one platform per native runner
variable "BASE_PLATFORMS" {
  default = "linux/amd64,linux/arm64"
}

# CI only: push untagged by digest; multi-arch tags are created later from `bake --print` without this flag
variable "PUSH_BY_DIGEST" {
  type    = bool
  default = false
}

function "tags" {
  params = [tag]
  result = PUSH_BY_DIGEST ? [] : ["${BASE_REPO}:${tag}"]
}

target "_base" {
  platforms = split(",", BASE_PLATFORMS)
  output    = PUSH_BY_DIGEST ? ["type=image,name=${BASE_REPO},push-by-digest=true,name-canonical=true,push=true"] : []
}

group "all" {
  targets = [
    "jvm-community", "jvm", "python-community", "python",
    "dotnet-community", "dotnet",
    "cpp-community", "cpp", "cpp-community-bookworm", "cpp-bookworm",
    "poly-base-latest", "go-base-latest", "js-base-latest", "php-base-latest", "rust-base-latest",
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

# POLY chain
group "poly" {
  targets = ["poly-base-latest"]
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
  inherits = ["_base"]
  tags = tags("jvm-community-base-latest")
  dockerfile = "jvm-community.Dockerfile"
}

target "jvm" {
  inherits = ["_base"]
  contexts = {
    jvm-community = "target:jvm-community"
  }
  tags = tags("jvm-base-latest")
  dockerfile = "jvm.Dockerfile"
}

target "python-community" {
  inherits = ["_base"]
  contexts = {
    jvm-community = "target:jvm-community"
  }
  tags = tags("python-community-base-latest")
  dockerfile = "python-community.Dockerfile"
}

target "python" {
  inherits = ["_base"]
  contexts = {
    python-community = "target:python-community"
  }
  tags = tags("python-base-latest")
  dockerfile = "python.Dockerfile"
}

target "poly-base-latest" {
  inherits = ["_base"]
  tags = tags("poly-base-latest")
  dockerfile = "poly.Dockerfile"
}

target "go-base-latest" {
  inherits = ["_base"]
  tags = tags("go-base-latest")
  dockerfile = "go.Dockerfile"
}

target "js-base-latest" {
  inherits = ["_base"]
  tags = tags("js-base-latest")
  dockerfile = "js.Dockerfile"
}

target "php-base-latest" {
  inherits = ["_base"]
  tags = tags("php-base-latest")
  dockerfile = "php.Dockerfile"
}

target "rust-base-latest" {
  inherits = ["_base"]
  tags = tags("rust-base-latest")
  dockerfile = "rust.Dockerfile"
}

target "cpp-community" {
  inherits = ["_base"]
  matrix = {
    clang = ["20", "21"]
  }
  name = "cpp-community-${clang}-latest"
  tags = tags("cpp-community-base-${clang}-latest")
  dockerfile = "cpp-community.Dockerfile"
  args = {
    CLANG = clang
  }
}

target "cpp" {
  inherits = ["_base"]
  contexts = {
    cpp-community = "target:cpp-community-${clang}-latest"
  }
  matrix = {
    clang = ["20", "21"]
  }
  name = "cpp-base-${clang}-latest"
  tags = tags("cpp-base-${clang}-latest")
  dockerfile = "cpp.Dockerfile"
  args = {
    CLANG = clang
  }
}

# Bookworm-based cpp for clang 16-19 (LLVM apt doesn't have these for Trixie)
target "cpp-community-bookworm" {
  inherits = ["_base"]
  matrix = {
    clang = ["16", "17", "18", "19"]
  }
  name = "cpp-community-bookworm-${clang}-latest"
  tags = tags("cpp-community-base-${clang}-latest")
  dockerfile = "cpp-community-bookworm.Dockerfile"
  args = {
    CLANG = clang
  }
}

target "cpp-bookworm" {
  inherits = ["_base"]
  contexts = {
    cpp-community = "target:cpp-community-bookworm-${clang}-latest"
  }
  matrix = {
    clang = ["16", "17", "18", "19"]
  }
  name = "cpp-base-bookworm-${clang}-latest"
  tags = tags("cpp-base-${clang}-latest")
  dockerfile = "cpp-bookworm.Dockerfile"
  args = {
    CLANG = clang
  }
}

target "dotnet-community" {
  inherits = ["_base"]
  tags = tags("dotnet-community-base-latest")
  dockerfile = "dotnet-community.Dockerfile"
}

target "dotnet" {
  inherits = ["_base"]
  contexts = {
    dotnet-community = "target:dotnet-community"
  }
  tags = tags("dotnet-base-latest")
  dockerfile = "dotnet.Dockerfile"
}

target "ruby3x" {
  inherits = ["_base"]
  matrix = {
    version = ["2", "3", "4"]
  }
  name = "ruby-base-3${version}"
  tags = tags("ruby-base-3.${version}-latest")
  dockerfile = "ruby.Dockerfile"
  args = {
    RUBY_TAG = "3.${version}-debian13-dev"
  }
}