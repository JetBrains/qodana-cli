# Contributing

By participating in this project, you agree to abide our [Code of conduct](.github/CODE_OF_CONDUCT.md).

## Set up your machine

Prerequisites:

- [Docker](https://docs.docker.com/get-docker/)

Other things you might need to develop:

- [IntelliJ IDEA](https://www.jetbrains.com/idea/) (it's [free for open-source development](https://www.jetbrains.com/community/opensource/))

Clone the project anywhere:

```sh
git clone git@github.com:JetBrains/qodana-docker.git
```

`cd` into the directory, with [Docker Bake](https://docs.docker.com/build/bake/) you can build all images at once:

```shell
docker buildx bake
```

## Create a commit

Commit messages should be well formatted, and to make that "standardized", we are using [internal issue tracker](https://youtrack.jetbrains.com) references.


## Submit a pull request

Push your branch to your repository fork and open a pull request against the
main branch.
