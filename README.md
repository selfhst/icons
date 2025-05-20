<div align="center">
  <img width="400" src="https://cdn.jsdelivr.net/gh/selfhst/cdn/assets/site/logos/selfh-st-icons.svg" alt="selfh.st/icons Logo">
</div>
<br/>
<p align="center">
<a href="https://selfh.st/support/#/portal/support">
<img src="https://img.shields.io/badge/Stripe-5469d4?style=for-the-badge&logo=stripe&logoColor=ffffff" alt="Stripe"/></a>
<a href="https://ko-fi.com/selfhst">
<img src="https://img.shields.io/badge/Ko--fi-F16061?style=for-the-badge&logo=ko-fi&logoColor=white" alt="Ko-fi"/></a>
<a href="https://patreon.com/selfhst">
<img src="https://img.shields.io/badge/Patreon-000000?style=for-the-badge&logo=patreon&logoColor=white" alt="Patreon"/></a>
<a href="https://buymeacoffee.com/selfhst">
<img src="https://img.shields.io/badge/Buy%20Me%20a%20Coffee-ffdd00?style=for-the-badge&logo=buy-me-a-coffee&logoColor=black" alt="Buy Me a Coffee"/></a>
</p>
<br/>

<div align="center">
  <img width="800" style="border-radius: 8px;" src="https://cdn.jsdelivr.net/gh/selfhst/cdn/assets/site/screenshots/selfh-st-icons.png" alt="selfh.st/icons Screenshot">
</div>
<br/>

[selfh.st/icons](https://selfh.st/icons) is a collection of 4,400+ logos and icons for self-hosted (and non-self-hosted) software.

The collection is available for browsing via the directory at [selfh.st/icons](https://selfh.st/icons) and served to users directly from this repo using the jsDelivr content delivery network.

To self-host the collection, users can clone, download, or sync the repository with a tool such [git-sync](https://github.com/AkashRajpurohit/git-sync) and serve it via a web server of their choosing (Caddy, NGINX, etc.).

## Color Options

By default, most SVG icons are available in three color formats:

* **Standard**: The standard colors of an icon without any modifications.
* **Dark**: A modified version of an icon displayed entirely in black (```#000000```). 
* **Light**: A modified version of an icon displayed entirely in white (```#FFFFFF```).

(Toggles to view icons by color type are available in the [directory hosted on the selfh.st website](https://selfh.st/icons).)

## Custom Colors

Because the dark and light versions of each icon are monochromatic, CSS can theoretically be leveraged to apply custom colors to the icons. 

This only works, however, when the SVG code is embedded directly onto a webpage. Unfortunately, most [integrations](https://selfh.st/apps/?tag=selfh-st-icons) link to the icons via an `<img>` tag, which prevents styling from being overridden via CSS.

As a workaround, a lightweight self-hosted server has been published via Docker that utilizes a URL parameter for color conversion on the fly. Continue reading for further instructions.


### Deploying the Custom Color Container

* [Introduction](https://github.com/selfhst/icons#introduction)
* [Deploying the container](https://github.com/selfhst/icons#deployment)
* [Configuring a reverse proxy (optional)](https://github.com/selfhst/icons#reverse-proxy)
* [Linking to a custom icon](https://github.com/selfhst/icons#linking)
* [Changelog](https://github.com/selfhst/icons#changelog)

#### Introduction

The Docker image below allows users to host a local server that acts as a proxy between requests and jsDelivr. When a color parameter is detected in the URL, the server will intercept the requests, fill the SVG file with that color, and serve it to the user.

Once deployed, users can append ```?color=eeeeee``` to the end of a URL to specify a custom color (replacing ```eeeeee``` with any [hex color code](https://htmlcolorcodes.com/)).

#### Deployment

The container can be easily deployed via docker-compose with the following snippet:

```
selfhst-icons:
  image: ghcr.io/selfhst/icons:latest
  restart: unless-stopped
  ports:
    - 4050:4050
```

No volume mounts or environment variables are currently required.

#### Reverse Proxy

While out of the scope of this guide, many applications will require users to leverage HTTPS when linking to icons served from the container.

The process to proxy the container and icons is straightforward. A sample Caddyfile configuration has been provided for reference:

```
icons.selfh.st {
	reverse_proxy selfhst-icons:4050
}
```

#### Linking

After the container has been deployed, users can easily link to any existing icon within the collection:

* ```https://icons.selfh.st/bookstack.svg```
* ```https://icons.selfh.st/bookstack.png```
* ```https://icons.selfh.st/bookstack-dark.webp```

To customize the color, users **must** link to the *standard* version of an SVG icon that has available monochromatic (dark/light) versions. To do so, append a custom URL parameter referencing any [hex color code](https://htmlcolorcodes.com/):

* ```https://icons.selfh.st/bookstack.svg?color=eeeeee```
* ```https://icons.selfh.st/bookstack.svg?color=439b68```

**Note the following:**

* Only the standard icons accept URL parameters (for example, ```bookstack-light.svg?color=fff000``` will not yield a different color.
* Only append the alpha-numeric portion of the hex color code to the URL. The server will append the ```#``` in the backend before passing it on for styling.

##### Changelog

* 2025-04-30: Initial release