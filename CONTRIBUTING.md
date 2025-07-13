# Contributing to Mac2MQTT

Thank you for your interest in contributing to Mac2MQTT! This document provides information on how to contribute to the project.

## Development Setup

### Prerequisites

- Go 1.21 or later
- macOS (for testing)
- Git

### Local Development

1. **Clone the repository**:
   ```bash
   git clone https://github.com/yourusername/mac2mqtt.git
   cd mac2mqtt
   ```

2. **Set up development environment**:
   ```bash
   make dev-setup
   ```

3. **Build the project**:
   ```bash
   make build
   ```

4. **Run tests**:
   ```bash
   make test
   ```

5. **Run locally**:
   ```bash
   make run
   ```

## Building for Different Architectures

### Local Building

Build for your current architecture:
```bash
make build
```

Build for both Intel and ARM Macs:
```bash
make build-all
```

Build for specific architecture:
```bash
make build-amd64    # Intel Mac
make build-arm64    # Apple Silicon Mac
```

### Creating Release Packages

Create release packages for both architectures:
```bash
make release
```

This will create:
- `mac2mqtt-darwin-amd64.tar.gz` (Intel Mac)
- `mac2mqtt-darwin-arm64.tar.gz` (Apple Silicon Mac)

## GitHub Actions Workflows

The project includes two GitHub Actions workflows:

### 1. Build and Test (`build.yml`)

**Triggers**: Push to main/master branch, Pull requests

**What it does**:
- Runs tests on macOS
- Builds for both Intel and ARM architectures
- Uploads build artifacts

**Artifacts**:
- `mac2mqtt-test` - Test build for current architecture
- `mac2mqtt-darwin-amd64` - Intel Mac binary
- `mac2mqtt-darwin-arm64` - Apple Silicon Mac binary

### 2. Release (`release.yml`)

**Triggers**: 
- Push of a tag starting with 'v' (e.g., `v1.0.0`)
- Manual workflow dispatch

**What it does**:
- Runs all tests
- Builds for both architectures
- Creates GitHub release with:
  - Release notes (auto-generated from git commits)
  - Binary packages for both architectures
  - Auto-installer script

## Creating a Release

### Automatic Release (Recommended)

1. **Create and push a tag**:
   ```bash
   git tag v1.0.0
   git push origin v1.0.0
   ```

2. **GitHub Actions will automatically**:
   - Build for both architectures
   - Create a GitHub release
   - Generate release notes
   - Upload binary packages

### Manual Release

1. Go to the GitHub repository
2. Click "Actions" tab
3. Select "Build and Release" workflow
4. Click "Run workflow"
5. Enter the version (e.g., `v1.0.0`)
6. Click "Run workflow"

## Release Artifacts

Each release includes:

### Binary Packages
- `mac2mqtt-darwin-amd64.tar.gz` - Intel Mac package
- `mac2mqtt-darwin-arm64.tar.gz` - Apple Silicon Mac package

### Auto-Installer
- `mac2mqtt-install.sh` - One-command installer script

### Package Contents
Each package contains:
- `mac2mqtt` - Main binary
- `mac2mqtt.yaml` - Configuration template
- `com.hagak.mac2mqtt.plist` - Launch agent
- `install.sh` - Installation script
- `uninstall.sh` - Uninstallation script
- `status.sh` - Status checking script
- `configure.sh` - Configuration script
- `debug.sh` - Debugging script
- `README.md` - Documentation
- `LICENSE` - License file
- `INSTALL.md` - Installation guide

## Testing

### Running Tests

```bash
make test
```

### Running Tests with Coverage

```bash
make dev-test
```

### Manual Testing

1. **Build and install**:
   ```bash
   make build install
   ```

2. **Check status**:
   ```bash
   make status
   ```

3. **Debug if needed**:
   ```bash
   make debug
   ```

4. **Uninstall**:
   ```bash
   make uninstall
   ```

## Code Quality

### Formatting

```bash
make format
```

### Linting

```bash
make lint
```

### Code Review Checklist

Before submitting a pull request, ensure:

- [ ] Code is formatted (`make format`)
- [ ] Linting passes (`make lint`)
- [ ] Tests pass (`make test`)
- [ ] Builds successfully for both architectures (`make build-all`)
- [ ] Documentation is updated
- [ ] Commit messages are clear and descriptive

## Commit Message Guidelines

Use conventional commit format:

```
type(scope): description

[optional body]

[optional footer]
```

Types:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `style`: Code style changes
- `refactor`: Code refactoring
- `test`: Test changes
- `chore`: Build/tooling changes

Examples:
```
feat(mqtt): add support for SSL connections
fix(volume): resolve volume control on Apple Silicon
docs(readme): update installation instructions
```

## Issues and Bug Reports

When reporting issues:

1. **Check existing issues** first
2. **Use the issue template** if available
3. **Include system information**:
   - macOS version
   - Architecture (Intel/Apple Silicon)
   - Go version
   - MQTT broker details
4. **Provide logs** from `./debug.sh`
5. **Describe steps to reproduce**

## Pull Request Process

1. **Fork the repository**
2. **Create a feature branch**:
   ```bash
   git checkout -b feature/your-feature-name
   ```
3. **Make your changes**
4. **Test thoroughly**:
   ```bash
   make test
   make build-all
   ```
5. **Commit with clear messages**
6. **Push to your fork**
7. **Create a pull request**
8. **Wait for review and CI checks**

## Release Process

### Pre-release Checklist

- [ ] All tests pass
- [ ] Documentation is up to date
- [ ] Version is updated in code (if applicable)
- [ ] Changelog is prepared
- [ ] Release notes are ready

### Release Steps

1. **Create release branch** (if needed):
   ```bash
   git checkout -b release/v1.0.0
   ```

2. **Update version** (if applicable)

3. **Commit changes**:
   ```bash
   git add .
   git commit -m "chore: prepare release v1.0.0"
   ```

4. **Create and push tag**:
   ```bash
   git tag v1.0.0
   git push origin v1.0.0
   ```

5. **Monitor GitHub Actions** for successful build and release

6. **Verify release** on GitHub

## Support

If you need help:

1. **Check the documentation** in README.md
2. **Search existing issues**
3. **Create a new issue** with detailed information
4. **Join discussions** in the GitHub repository

Thank you for contributing to Mac2MQTT! 🚀 