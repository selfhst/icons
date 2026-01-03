# v3.0.0

## Breaking Changes

The legacy custom color URL ```domain.com/icon.svg?color=000000``` is no longer supported. See the [supported methods for building URLs](https://github.com/selfhst/icons/wiki#building-links) in the project's wiki if this change impacts you.

## What's Changed

* Added support for the new AVIF and ICO formats
* Updated Go version to [v1.25](https://go.dev/doc/go1.25)
* Removed unmaintained [gorilla/mux](https://github.com/gorilla/mux) external dependency in favor of [net/http enhancements introduced in Go 1.22](https://go.dev/blog/routing-enhancements)

# v2.2.0

## What's Changed

* Remote source now points to main branch to ensure latest icons are fetched

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