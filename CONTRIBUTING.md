# Contributing to KubeVirt Migrator

This repository contains the KubeVirt Migrator CLI tool for migrating virtual machines between OpenShift/Kubernetes clusters. This guide outlines how to contribute to the project.

## Issue Reporting

We encourage users to report:
* CLI tool bugs
* Performance issues
* Documentation improvements
* Feature requests

Please use the appropriate issue template when creating new issues.

## Development Environment

### Prerequisites

* Go 1.23 or later
* Docker (for building container images)
* Access to OpenShift/Kubernetes clusters with KubeVirt for testing
* [Task](https://taskfile.dev/) - Task runner tool (used instead of Makefiles)

### Local Development Setup

1. Clone the repository:
   ```bash
   git clone https://github.com/kloia/kubevirt-migrator.git
   cd kubevirt-migrator
   ```

2. Install dependencies:
   ```bash
   task download
   ```

3. Run linting:
   ```bash
   task lint
   ```

4. Build the binary:
   ```bash
   task build
   ```

5. Run tests:
   ```bash
   task test
   ```

All available tasks can be viewed in the [Taskfile.yaml](Taskfile.yaml) or by running:
```bash
task --list
```

## Pull Requests

We welcome pull requests for:
* Bug fixes
* Feature implementations
* Documentation improvements
* Test enhancements

### Pull Request Process

1. Fork the repository and create a new branch
2. Make your changes
3. Add/update tests as necessary
4. Ensure all tests pass and linting is clean
5. Submit a pull request
6. Sign all your commits using `git commit -s` or `git commit --signoff`

### Pull Request Title Format

We follow the [Conventional Commits](https://www.conventionalcommits.org/) specification for PR titles. The format should be:

```
<type>(<scope>): <description>
```

Where:
- **type** is one of: feat, fix, docs, style, refactor, test, chore
- **scope** is the area of the project being modified (e.g., cli, replication, migration)
- **description** is a brief description of the changes

Examples:
- `feat(cli): Add support for custom SSH port`
- `fix(replication): Fix issue with large disk synchronization`
- `docs(readme): Update installation instructions`

## Versioning

KubeVirt Migrator follows [Semantic Versioning](https://semver.org/):

* **Major version (X.y.z)**: Incompatible API changes
* **Minor version (x.Y.z)**: Backward-compatible new functionality
* **Patch version (x.y.Z)**: Backward-compatible bug fixes

### Release Process

Releases are created by:

1. Creating a new tag following the pattern `vX.Y.Z` (e.g., `v1.0.0`)
2. Pushing the tag to the repository
3. The GitHub Actions workflow will automatically build and publish the release

```bash
# Create and push a new tag
git tag -a v1.0.0 -m "Release v1.0.0"
git push origin v1.0.0
```

The release workflow will:
1. Build binaries for multiple platforms using GoReleaser
2. Create a GitHub release with release notes
3. Attach the binaries to the GitHub release

## Testing

Before submitting a PR, please ensure:

1. All unit tests pass:
   ```bash
   task test
   ```

2. Integration tests pass (if applicable):
   ```bash
   go test ./test/integration/...
   ```

3. The code passes linting:
   ```bash
   task lint
   ```

4. Manual testing with both source and destination clusters has been performed for functionality changes.

## Code Style

We follow standard Go coding conventions:

* Use `gofmt` to format your code
* Follow the [Effective Go](https://golang.org/doc/effective_go) guidelines
* Include comments for exported functions, types, and packages
* Write clear and concise commit messages

## Documentation

For any change that affects user-facing behavior, please update:

1. The README.md file if installation or basic usage instructions change
2. Command help text for CLI changes
3. Any relevant examples

## License

By contributing to this project, you agree that your contributions will be licensed under the project's [MIT License](LICENSE). 