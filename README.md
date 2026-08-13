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

A wrapper around the [`go-mutesting`](https://github.com/avito-tech/go-mutesting) tool that adds intelligent filtering, dynamic line shifting, and self-healing ignore capabilities. 

It generates a mutation testing report while automatically blacklisting mutations that are either explicitly ignored via a `.go-mutesting-ignore` file or that fall outside of your current test coverage boundaries, heavily reducing noise and false positives.

**Advanced Features:**
*   **Intelligent Shifting:** You can manually add a mutant to the `.go-mutesting-ignore` file. As you add or remove lines of code in your repository, the tool uses Git history to calculate diffs and automatically shifts the line numbers of your ignored mutations, keeping them perfectly synced with the codebase.
*   **Self-Healing:** At the end of a run, the tool performs a secondary verification pass on your ignored mutations. If your test suite has improved and is now capable of killing an ignored mutation, the tool will automatically "self-heal" by removing that mutation from the ignore file. 

**Ignore File Format:**
The tool expects a `.go-mutesting-ignore` file in the root of your project. You can copy the exact format output by the mutation HTML report:
```
# format: filepath:line:mutatorName
internal/app/syncer.go:42:branch/if
internal/app/syncer.go:48:statement/remove
```

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
