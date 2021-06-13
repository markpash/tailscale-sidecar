# tailscale-sidecar

[![Publish docker image](https://github.com/markpash/tailscale-sidecar/actions/workflows/push-image.yml/badge.svg)](https://github.com/markpash/tailscale-sidecar/actions/workflows/push-image.yml)

This is barely tested software, I don't guarantee it works but please make an issue if you use it and find a bug. Pull requests are welcome.

This program is designed to expose services onto a tailscale network without needing root.
Using the `tsnet` package provided by tailscale, we can listen on a port on a tailscale IP and then proxy the stream to a destination.
The use-case for me was running this as a sidecar container in nomad to expose services onto my tailscale network, without needing root or routing.

Currently this only supports tcp because right now because that's all I care about. I may try to make UDP work in the future.

Docker image available:

```bash
docker pull ghcr.io/markpash/tailscale-sidecar:latest
```

## Usage

To use this program, it needs to be executed with a few environment variables. They are as follows:

```bash
TS_LOGIN
TS_SIDECAR_STATEDIR
TS_SIDECAR_NAME
TS_SIDECAR_BINDINGS
```

On first run, you should run with `TS_LOGIN` set to `1` as this will output a login url in the logs, which you can use to authorise this instance and create the state. Once the instance is authorised, the variable no longer needs to be provided.

`TS_SIDECAR_STATEDIR` is the location where the persistent data for the sidecar will be stored. This is used to not need to re-authorise the instance. In a container setup, you'll want to have this persisted. The default path is `./tsstate`.

`TS_SIDECAR_NAME` is the name that you wish this program to use to present itself to the tailscale servers, this is what you will see in your panel.

`TS_SIDECAR_BINDINGS` is the path to the bindings file, which should be a JSON file which has contents much like what's below.
The default path for bindings is `/etc/ts-sidecar/bindings.json`.

## Configuration

Configuration should look like this:

```json
[
    {
        "from": 80,
        "to": "127.0.0.1:8000"
    }
]
```

## Disclaimer

THIS IS NOT OFFICIALLY ENDORSED BY TAILSCALE.

I thought I should put that there just in case someone thought it may be a tailscale product.
I'm also not responsible for any of the bad things that might happen as a result of using this software. It works for me but maybe not for you.
