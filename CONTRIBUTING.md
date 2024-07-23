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

`cd` into `.github/scripts` and run the script to check product feed if you edited something in `feed/releases.json`:

```shell
cd .github/scripts && node verifyChecksums.js
```

## Create a commit

Commit messages should be well formatted, and to make that "standardized", we are using [internal issue tracker](https://youtrack.jetbrains.com) references.


## Submit a pull request

Push your branch to your repository fork and open a pull request against the
main branch.

## Create a new directory for a new release branch

Run the script `release_branch.sh` with the release version as an argument. For example, to create a new directory for the 2024.3 release branch:

```shell
./release_branch.sh 2024.3
```
