# v4.0.2

## What's Changed

* Fixed custom icon issue where names converted to lowercase for cache keys were also being used to look up the file path, causing requests for icons with uppercase characters to fail ([#751](https://github.com/selfhst/icons/issues/751))
* Added server name and sample endpoints to the root path ([#752](https://github.com/selfhst/icons/issues/752))

# v4.0.1

## What's Changed

* Replaced hard-coded cache age values with the `CACHE_TTL` variable added in v4.0.0 ([#748](https://github.com/selfhst/icons/issues/748))
* Updated background routine to periodically purge stale assets based on `CACHE_TTL` time

# v4.0.0

This release initially set out to address a request to support icon extensions in URLs, but quickly grew into a number of under the hood optimization/security fixes and a new ```hybrid``` mode that allows users with local collections to set remote icons as a fallback.

## Breaking Changes

The ```STANDARD_ICON_FORMAT``` variable has been deprecated and users now have the option to specify their desired icon extension in the URL. If an extension is not specified, the server will default to WebP.

Users should review the updated methods for referencing icons in the [project's wiki](https://github.com/selfhst/icons/wiki#building-links) when upgrading.

## What's Changed

* [Feature] Added support for icon extensions in the URL (```example.svg```, ```example.png```, etc.) ([#718](https://github.com/selfhst/icons/issues/718), [#737](https://github.com/selfhst/icons/issues/737))
* [Feature] Added support for custom color URL parameters (```?color=2d2d2d```) as an alternative to paths (```icon/2d2d2d```) ([#737](https://github.com/selfhst/icons/issues/737))
* [Feature] Added a new ```hybrid``` source option that prioritizes local collections and falls back to remote when missing
* [Feature] Added an optional ```LOG_LEVEL``` variable to control log verbosity
* [Feature] Added an optional ```CORS_ALLOWED_ORIGINS``` variable (defaults to all or ```*```)
* [Feature] Added optional variables for configurable cache times (```CACHE_TTL```) and size (```CACHE_SIZE```)
* [Feature] Added an optional ```REMOTE_TIMEOUT``` variable to prevent the server from hanging when requests take too long (default: ```10``` seconds)
* Configured WebP as the global default when no extension is specified or an unsupported extension is requested
* Added Docker health checks via the `-healthcheck` flag to accommodate scratch's lack of tooling (curl/wget)
* Added startup validations and logging to warn users of misconfigured variables
* Added graceful shutdowns to allow in-flight requests to complete before exiting
* Added validations to properly identify ```#fff``` and ```#ffffff``` when colorizing icons
* Added request completion time to log messages
* Added case normalization checks to prevent the server from caching the same icon multiple times
* Added GZIP compression for SVG requests with proper proxy headers
* Added a security check for icons names with ```/```, ```\```, or ```..``` (attempted path traversal)
* Added an ```X-Cache``` response header to signal cache status without having to view container logs
* Added a process to regularly clean stale cache entries
* Removed redundant file existence checks for local icons
* Removed unnecessary directory scans when a custom icon is not found (initially included for debugging purposes)

# v3.2.0

## What's Changed

* [Feature] Added ```PRIMARY_COLOR``` variable to easily apply a single custom color to all icons (see Wiki for additional details)
* [Feature] Added ```REMOTE_URL``` to allow users to serve icons from their own remote sources (see Wiki for additional details) ([#690](https://github.com/selfhst/icons/issues/690))
* Reduced remote icon load time by removing redundant existence checks
* Updated Go version to [v1.26](https://go.dev/doc/go1.26)

# v3.1.1

## What's Changed

* Fixed crash caused by concurrent writes under high load ([#684](https://github.com/selfhst/icons/issues/684))
* Suppress favicon error log message when viewing icons directly from a browser

# v3.1.0

## What's Changed

* Updated logic to also replace gradient fills with custom colors when applicable

# v3.0.0

## Breaking Changes

The legacy custom color URL ```domain.com/icon.svg?color=000000``` is no longer supported. See the [supported methods for building URLs](https://github.com/selfhst/icons/wiki#building-links) in the project's wiki if this change impacts you.

## What's Changed

* [Feature] Added support for the new AVIF and ICO formats
* Updated Go version to [v1.25](https://go.dev/doc/go1.25)
* Removed unmaintained [gorilla/mux](https://github.com/gorilla/mux) external dependency in favor of [net/http enhancements introduced in Go 1.22](https://go.dev/blog/routing-enhancements)

# v2.2.0

## What's Changed

* Remote source now points to main branch to ensure latest icons are fetched

# v2.1.0

## What's Changed

* [Feature] Added optional volume for custom (non-selfh.st) icons ([#495](https://github.com/selfhst/icons/issues/495))

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