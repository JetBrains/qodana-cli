group "default" {
  targets = ["232"]
}

target "232" {
  name = "qodana-${edition}"
  matrix = {
    edition = ["android-community", "dotnet", "go", "js", "jvm", "jvm-community", "php", "python", "python-community"]
    version = ["2023.2"]
  }
  args = {
    QD_RELEASE = "2023.2"
    BASE_TAG = "bullseye-slim"
    DOTNET_TAG = "6.0-bullseye-slim"
    GO_TAG = "1.21-bullseye"
    NODE_TAG = "16-bullseye-slim"
    PHP_TAG = "8.2-cli-bullseye"
    COMPOSER_TAG="2.6.3"
  }
  tags = [
    "docker.io/jetbrains/qodana:${edition}-${version}-latest"
  ]
#  platforms = ["linux/amd64", "linux/arm64"]  // uncomment to build for multiple platforms, but it will take more time
  dockerfile = "${version}/${edition}/Dockerfile"
}