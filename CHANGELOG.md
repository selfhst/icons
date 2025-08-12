# v2.1.0

## What's Changed

* Added optional volume for custom (non-selfh.st) icons ([#495](https://github.com/selfhst/icons/issues/495))

# v2.0.0

## Breaking Changes

This release contains breaking changes as the image was rewritten using Go to decrease size and accommodate new features:

* Updated docker-compose file with new variables and volume mounts
* Docker Hub image has been deprecated and will no longer receive updates

## What's Changed

* Reduced image size (125MB --> 8MB) after replacing Node.js with Go
* Support for local icon files
* Volume mounts for users hosting icon files locally
* Implemented caching to reduce external requests for commonly used assets
* Detailed logging

# v1.0.0

* Initial release