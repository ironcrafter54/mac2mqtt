# GitHub Actions Setup for Mac2MQTT

This document provides a quick reference for the GitHub Actions workflows that have been set up for Mac2MQTT.

## Workflows Overview

### 1. Build and Test (`build.yml`)
- **Triggers**: Push to main/master, Pull requests
- **Purpose**: Continuous integration testing
- **Outputs**: Build artifacts for both Intel and ARM Macs

### 2. Release (`release.yml`)
- **Triggers**: Git tags (v*), Manual dispatch
- **Purpose**: Create GitHub releases
- **Outputs**: Release packages, auto-installer script

## Quick Start

### For Development
1. Push to main branch → Automatic build and test
2. Check Actions tab for results

### For Releases
1. Create and push a tag:
   ```bash
   git tag v1.0.0
   git push origin v1.0.0
   ```
2. GitHub Actions will automatically create a release

### Manual Release
1. Go to Actions → Build and Release
2. Click "Run workflow"
3. Enter version (e.g., `v1.0.0`)
4. Click "Run workflow"

## Local Development

### Build Commands
```bash
make build          # Build for current architecture
make build-all      # Build for both Intel and ARM
make build-amd64    # Build for Intel Mac
make build-arm64    # Build for Apple Silicon Mac
```

### Testing
```bash
make test           # Run tests
make dev-test       # Run tests with coverage
make lint           # Run linting
```

### Release Creation
```bash
make release        # Create release packages locally
```

## Release Artifacts

Each release includes:

### Binary Packages
- `mac2mqtt-darwin-amd64.tar.gz` (Intel Mac)
- `mac2mqtt-darwin-arm64.tar.gz` (Apple Silicon Mac)

### Auto-Installer
- `mac2mqtt-install.sh` (One-command installer)

### Package Contents
- Main binary (`mac2mqtt`)
- Configuration template (`mac2mqtt.yaml`)
- Launch agent (`com.hagak.mac2mqtt.plist`)
- Installation script (`install.sh`)
- Status script (`status.sh`)
- Debug script (`debug.sh`)
- Documentation (`README.md`, `INSTALL.md`)

## Installation Methods

### Automatic Installation
```bash
curl -L https://github.com/yourusername/mac2mqtt/releases/download/v1.0.0/mac2mqtt-install.sh | bash
```

### Manual Installation
1. Download appropriate package for your Mac
2. Extract: `tar -xzf mac2mqtt-darwin-*.tar.gz`
3. Run: `./install.sh`

## Architecture Support

- **Intel Macs (x86_64)**: `darwin-amd64`
- **Apple Silicon Macs (arm64)**: `darwin-arm64`

## Workflow Features

### Automatic Features
- Cross-platform building (Intel + ARM)
- Automated testing
- Release note generation from git commits
- Auto-installer script creation
- GitHub release creation

### Manual Features
- Manual workflow dispatch
- Custom version input
- Build artifact downloads

## Troubleshooting

### Common Issues
1. **Build fails**: Check Go version compatibility
2. **Tests fail**: Ensure all dependencies are available
3. **Release fails**: Verify tag format (must start with 'v')

### Debug Commands
```bash
make debug          # Run debug script
make status         # Check service status
```

## Next Steps

1. **Push to GitHub**: All workflows are ready to use
2. **Create first release**: Use `git tag v1.0.0 && git push origin v1.0.0`
3. **Monitor Actions**: Check the Actions tab for build status
4. **Test releases**: Download and test the generated packages

## Files Created

- `.github/workflows/build.yml` - CI/CD workflow
- `.github/workflows/release.yml` - Release workflow
- `Makefile` - Local development commands
- `CONTRIBUTING.md` - Development guide
- `GITHUB_ACTIONS_SETUP.md` - This file

The setup is complete and ready for use! 🚀 