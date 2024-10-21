The base images are used to build the internal Qodana images. They are not intended to be used directly, but are built regularly to minimize the build time of the Qodana images.

The [images are built on GitHub](https://github.com/JetBrains/qodana-docker/actions) and pushed to [our internal registry](https://jetbrains.team/p/sa/packages/container/containers).

To build them all locally, you can run the following command

```shell
docker buildx bake
```

With `--print` option in the previous command you'll get the full picture of images we do build:

```json
{
  "group": {
    "default": {
      "targets": [
        "debian",
        "debian-js",
        "python",
        "python-js",
        "other"
      ]
    },
    "other": {
      "targets": [
        "dotnet-base",
        "go-base",
        "js-base",
        "php-base",
        "rust-base",
        "ruby-base"
      ]
    }
  },
  "target": {
    "debian": {
      "context": ".",
      "dockerfile": "debian.Dockerfile",
      "tags": [
        "registry.jetbrains.team/p/sa/containers/qodana:debian-base"
      ],
      "platforms": [
        "linux/amd64",
        "linux/arm64"
      ]
    },
    "debian-js": {
      "context": ".",
      "contexts": {
        "debianbase": "target:debian"
      },
      "dockerfile": "debian.js.Dockerfile",
      "args": {
        "NODE_TAG": "20-bullseye-slim"
      },
      "tags": [
        "registry.jetbrains.team/p/sa/containers/qodana:debian-js-base"
      ],
      "platforms": [
        "linux/amd64",
        "linux/arm64"
      ]
    },
    "dotnet-base": {
      "context": ".",
      "dockerfile": "dotnet.Dockerfile",
      "args": {
        "COMPOSER_TAG": "2.5.1",
        "DOTNET_TAG": "6.0-bullseye-slim",
        "GO_TAG": "1.19-bullseye",
        "NODE_TAG": "20-bullseye-slim",
        "PHP_TAG": "8.1-cli-bullseye",
        "RUBY_TAG": "3.0-bullseye",
        "RUST_TAG": "1.71-slim-bullseye"
      },
      "tags": [
        "registry.jetbrains.team/p/sa/containers/qodana:dotnet-base"
      ],
      "platforms": [
        "linux/amd64",
        "linux/arm64"
      ]
    },
    "go-base": {
      "context": ".",
      "dockerfile": "go.Dockerfile",
      "args": {
        "COMPOSER_TAG": "2.5.1",
        "DOTNET_TAG": "6.0-bullseye-slim",
        "GO_TAG": "1.19-bullseye",
        "NODE_TAG": "20-bullseye-slim",
        "PHP_TAG": "8.1-cli-bullseye",
        "RUBY_TAG": "3.0-bullseye",
        "RUST_TAG": "1.71-slim-bullseye"
      },
      "tags": [
        "registry.jetbrains.team/p/sa/containers/qodana:go-base"
      ],
      "platforms": [
        "linux/amd64",
        "linux/arm64"
      ]
    },
    "js-base": {
      "context": ".",
      "dockerfile": "js.Dockerfile",
      "args": {
        "COMPOSER_TAG": "2.5.1",
        "DOTNET_TAG": "6.0-bullseye-slim",
        "GO_TAG": "1.19-bullseye",
        "NODE_TAG": "20-bullseye-slim",
        "PHP_TAG": "8.1-cli-bullseye",
        "RUBY_TAG": "3.0-bullseye",
        "RUST_TAG": "1.71-slim-bullseye"
      },
      "tags": [
        "registry.jetbrains.team/p/sa/containers/qodana:js-base"
      ],
      "platforms": [
        "linux/amd64",
        "linux/arm64"
      ]
    },
    "php-base": {
      "context": ".",
      "dockerfile": "php.Dockerfile",
      "args": {
        "COMPOSER_TAG": "2.5.1",
        "DOTNET_TAG": "6.0-bullseye-slim",
        "GO_TAG": "1.19-bullseye",
        "NODE_TAG": "20-bullseye-slim",
        "PHP_TAG": "8.1-cli-bullseye",
        "RUBY_TAG": "3.0-bullseye",
        "RUST_TAG": "1.71-slim-bullseye"
      },
      "tags": [
        "registry.jetbrains.team/p/sa/containers/qodana:php-base"
      ],
      "platforms": [
        "linux/amd64",
        "linux/arm64"
      ]
    },
    "python": {
      "context": ".",
      "contexts": {
        "debianbase": "target:debian"
      },
      "dockerfile": "python.Dockerfile",
      "tags": [
        "registry.jetbrains.team/p/sa/containers/qodana:python-base"
      ],
      "platforms": [
        "linux/amd64",
        "linux/arm64"
      ]
    },
    "python-js": {
      "context": ".",
      "contexts": {
        "pythonbase": "target:python"
      },
      "dockerfile": "python.js.Dockerfile",
      "args": {
        "NODE_TAG": "20-bullseye-slim"
      },
      "tags": [
        "registry.jetbrains.team/p/sa/containers/qodana:python-js-base"
      ],
      "platforms": [
        "linux/amd64",
        "linux/arm64"
      ]
    },
    "ruby-base": {
      "context": ".",
      "dockerfile": "ruby.Dockerfile",
      "args": {
        "COMPOSER_TAG": "2.5.1",
        "DOTNET_TAG": "6.0-bullseye-slim",
        "GO_TAG": "1.19-bullseye",
        "NODE_TAG": "20-bullseye-slim",
        "PHP_TAG": "8.1-cli-bullseye",
        "RUBY_TAG": "3.0-bullseye",
        "RUST_TAG": "1.71-slim-bullseye"
      },
      "tags": [
        "registry.jetbrains.team/p/sa/containers/qodana:ruby-base"
      ],
      "platforms": [
        "linux/amd64",
        "linux/arm64"
      ]
    },
    "rust-base": {
      "context": ".",
      "dockerfile": "rust.Dockerfile",
      "args": {
        "COMPOSER_TAG": "2.5.1",
        "DOTNET_TAG": "6.0-bullseye-slim",
        "GO_TAG": "1.19-bullseye",
        "NODE_TAG": "20-bullseye-slim",
        "PHP_TAG": "8.1-cli-bullseye",
        "RUBY_TAG": "3.0-bullseye",
        "RUST_TAG": "1.71-slim-bullseye"
      },
      "tags": [
        "registry.jetbrains.team/p/sa/containers/qodana:rust-base"
      ],
      "platforms": [
        "linux/amd64",
        "linux/arm64"
      ]
    }
  }
}
```