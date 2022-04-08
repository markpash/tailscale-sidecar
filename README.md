# tailscale-sidecar

[![checks](https://github.com/markpash/tailscale-sidecar/actions/workflows/checks.yml/badge.svg)](https://github.com/markpash/tailscale-sidecar/actions/workflows/checks.yml)

This program is designed to expose services onto a tailscale network without needing root. Using the `tsnet` package provided by tailscale, we can listen on a port on a tailscale IP and then proxy the stream to a destination. The use-case for me was running this as a sidecar container in nomad to expose services onto my tailscale network, without needing root or routing.

Currently this only supports tcp because right now because that's all I care about. I may try to make UDP work in the future.

Docker image available:

```bash
docker pull ghcr.io/markpash/tailscale-sidecar:latest
```

Versions of this software track the versions of upstream tailscale. Any features added to this software will be released when the next version of tailscale is released.

## Usage

To use this program, it needs to be executed with a few environment variables. They are as follows:

```bash
TS_AUTHKEY
TS_SIDECAR_STATEDIR
TS_SIDECAR_NAME
TS_SIDECAR_BINDINGS
```

`TS_AUTHKEY` is now enabled for this project. You can provide this variable with a key, consult the tailscale documentation to determine the appropriate key to use. The old `TS_LOGIN` method still works, but it's not advised and it's not very convenient either.

`TS_SIDECAR_STATEDIR` is the location where the persistent data for the sidecar will be stored. This is used to not need to re-authorise the instance. In a container setup, you'll want to have this persisted. The default is `./tsstate`, which will result in Tailscale using `home/nonroot/tsstate` in the Docker container.

⚠ Tailscale will not use the specified state directory to store the TLS certificates. When using the Docker container, you should mount `home/nonroot/.local/share/tailscale`.

`TS_SIDECAR_NAME` is the name that you wish this program to use to present itself to the tailscale servers, this is what you will see in your panel.

`TS_SIDECAR_BINDINGS` is the path to the bindings file, which should be a JSON file which has contents much like what's below.
The default path for bindings is `/etc/ts-sidecar/bindings.json`.

## Configuration

Configuration should look like this:

```json
[
    {
        "from": 443,
        "to": "127.0.0.1:8000",
        "tls": true
    }
]
```

There is also support for HTTP proxying:

```json
[
    {
        "from": 443,
        "to": "http://127.0.0.1:8000/",
        "tls": true,
        "protocol": "http",
        "http": {
            "host": "localhost",
            "headers": {
                "User-Agent": "SomeAgent/2.0"
            }
        }
    }
]
```


## Disclaimer

THIS IS NOT OFFICIALLY ENDORSED BY TAILSCALE.

I thought I should put that there just in case someone thought it may be a tailscale product.
I'm also not responsible for any of the bad things that might happen as a result of using this software. It works for me but maybe not for you.
