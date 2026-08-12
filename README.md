# go-tools

A collection of custom Go development and testing tools to improve productivity, code quality, and testing insights.

## Features & Tools

### 1. `Makefile` & `.golangci.yml`

This repository provides a centralized `Makefile` and `.golangci.yml` configuration. The `Makefile` orchestrates fetching the latest configurations, installing necessary tools, formatting code, running tests, checking coverage diffs, and executing mutation testing.

### 2. `cover-diff`

A utility to compare test coverage between your current branch and a base branch (or commit) and report any regressions or uncovered new code. It generates and parses base coverage on the fly and compares it against your local coverage profile to ensure new code meets testing standards.

**Usage:**

```sh
cover-diff -coverprofile <path-to-coverage.out> [-base <branch-or-commit>]
```

### 3. `go-mutesting-ignore`

A wrapper around the [`go-mutesting`](https://github.com/avito-tech/go-mutesting) tool that adds intelligent filtering and ignore capabilities. It generates a mutation testing report while automatically blacklisting mutations that are either explicitly ignored via a `.go-mutesting-ignore` file or that fall outside of your current test coverage boundaries. 

This helps focus mutation testing efforts only on lines of code that are supposed to be covered, reducing noise and false positives.

**Usage:**

```sh
go-mutesting-ignore -coverprofile <path-to-coverage.out>
```

## Setup & Usage

To use these tools and configurations in a new repository, you only need to copy the `Makefile` over. The `Makefile` will handle downloading the latest configurations and installing the required CLI tools.

1. Download the `Makefile` to your project root:
   ```sh
   curl -sSfL https://raw.githubusercontent.com/danrneal/go-tools/main/Makefile -o Makefile
   ```

2. Run the complete pipeline to install tools, fetch configs, and run all checks:
   ```sh
   make all
   ```

3. For faster development cycles (skipping tool updates and full mutation testing), use:
   ```sh
   make fast
   ```

## License

go-tools is licensed under the [MIT License](LICENSE).
