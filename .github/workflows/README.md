# GitHub Actions Workflows

This directory contains GitHub Actions workflows for the Notifuse project.

## Docker Workflows

### 1. Docker Release Workflow (`docker-release.yml`)

**Trigger:** Automatically triggered when you push a tag that matches `v*.*` pattern or the `latest` tag.

**Features:**

- Builds multi-architecture Docker images (linux/amd64, linux/arm64)
- Pushes to Docker Hub (docker.io)
- Creates multiple tags based on semantic versioning:
  - `v1.2.3` (exact version from tag)
  - `v1.2` (major.minor)
  - `v1` (major)
  - `latest` (when pushing `latest` tag)
- Uses GitHub Actions cache for faster builds
- Generates build attestations for security
- Supports Docker Hub authentication

**Usage:**

1. Push a version tag: `git push origin v1.2.3`
2. Or push the latest tag: `git push origin latest`
3. The workflow will automatically build and push the Docker image
4. The image will be available at `your-dockerhub-username/notifuse`

### 2. Manual Docker Workflow (`docker-manual.yml`)

**Trigger:** Manual workflow dispatch from GitHub Actions tab.

**Features:**

- Allows manual triggering for testing
- Configurable tag input
- Option to build without pushing
- Same multi-architecture support as release workflow
- Useful for testing builds before releases

**Usage:**

1. Go to Actions tab in GitHub
2. Select "Docker Build and Push (Manual)"
3. Click "Run workflow"
4. Configure:
   - **Tag**: Custom tag (defaults to 'latest')
   - **Push**: Whether to push to registry (defaults to true)

## Required Secrets

The workflows require Docker Hub authentication secrets:

- `DOCKERHUB_USERNAME` - Your Docker Hub username
- `DOCKERHUB_TOKEN` - Your Docker Hub access token (not password)

## Registry Configuration

The workflows are configured to use Docker Hub (`docker.io`). To use a different registry:

1. Update the `REGISTRY` environment variable in both workflow files
2. Add appropriate secrets for authentication
3. Update the login step with the correct credentials

## Image Tags

### Release Workflow Tags

- `v1.2.3` - Exact version from release tag
- `v1.2` - Major.minor version
- `v1` - Major version only
- `latest` - Latest release (if on main branch)

### Manual Workflow Tags

- Custom tag specified in workflow input
- Defaults to `latest` if no tag provided

## Multi-Architecture Support

Both workflows build for:

- `linux/amd64` - Intel/AMD 64-bit
- `linux/arm64` - ARM 64-bit (Apple Silicon, ARM servers)

## Caching

The workflows use GitHub Actions cache to speed up builds:

- **Cache from**: Previous builds
- **Cache to**: Current build artifacts
- **Mode**: Maximum cache usage

## Security Features

- **Build attestations**: Generated for each build
- **Provenance**: Links builds to source code
- **Registry authentication**: Uses GitHub's built-in token system

## Troubleshooting

### Common Issues

1. **Permission denied**: Ensure the repository has the correct permissions for packages
2. **Build failures**: Check the Dockerfile and ensure all dependencies are available
3. **Push failures**: Verify the `GITHUB_TOKEN` has package write permissions

### Debugging

- Check the Actions tab for detailed logs
- Use the manual workflow to test builds without creating releases
- Verify Dockerfile syntax and dependencies

## Example Usage

### Creating a Release

```bash
# Tag your release
git tag -a v1.2.3 -m "Release version 1.2.3"
git push origin v1.2.3

# Or update the latest tag
git tag -f latest
git push origin latest
```

### Pulling the Image

```bash
# Pull the latest release
docker pull your-dockerhub-username/notifuse:latest

# Pull a specific version
docker pull your-dockerhub-username/notifuse:v1.2.3

# Run the container
docker run -p 8080:8080 your-dockerhub-username/notifuse:latest
```
