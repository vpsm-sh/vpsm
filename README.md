# VPSM
A CLI tool for managing VPS instances across cloud providers.
![VPSM Logo - A Beaver with a server on his back](docs/images/TransparentLogo.png)

## Usage
```
vpsm auth login hetzner
vpsm auth login vultr

vpsm config set default-provider hetzner
vpsm server create --name my-server --image ubuntu-24.04 --type cpx11 --location fsn1
vpsm server create --provider vultr --name my-server --image 2284 --type vc2-1c-2gb --location ewr
```

Supported server providers: Hetzner, Vultr.

## Examples
### Server create
![Server create](docs/images/Create-Demo.png)

### Server list
![Server list](docs/images/Show-Demo.png)
