# `crfl`: cloud reflection

## What is `crfl`?

`crfl` is a simple relay tool that allows you to build connection between clients behind NATs and/or firewalls.

## Features

### Connection

`crfl` clients need a server to help build connection:

```bash
crfls -p 3862  # listen on port 3862; this is the port by default
```

Suppose an http server is listening on 80 on machine A, and machine B wants to transfer from `:8080` on B to `:80` on A.
On machine A you run

```bash
crflc -s x.x.x.x -p 3862 -f 80  # x.x.x.x is server ip
```

On machine B you run

```bash
crflc -s x.x.x.x -p 3862 -l 8080
```

`crfl` supports `tcp/udp`.

### Multiple clients

Each `crfls` holds a network that transfers data between `crflc`s. By default there can be multiple `crflc` connecting
to one `crfls`, but there can only be one `crflc` listening on the network. In this case all datapacks received from
other `crflc`s will be transferred to the listening `crflc`.

You can allow multiple listening clients on `crfls` by running

```bash
crfls -n 10  # this allows 10 listening clients at most
```

In this case when running

```bash
crflc -s x.x.x.x -l 7000
```

`crflc` will listen on a range of ports: `7000-7009` to match each potential listening client on the network.

You can also specify the port for each listening client separately by running

```bash
crflc -s x.x.x.x -l 7000,7010,7020,8000
```

In this case `crflc` gives `7000` to the first listening client, `7010` to the second, `7020` to the third, and
`8000-8006` to the rest.

### Specifying listening port

Sometimes connection breaks, and when the listening client is reconnected, it may be given a different port on the
network, causing some existing work to unmatch. In this case, you can specify the port to be listened on by running

```bash
crflc -s x.x.x.x -f 80 -i 5
```

In this case `crflc` listens on the fifth port specified on each normal client. For example,

```bash
crflc -s x.x.x.x -l 7000,7010,7020,8000
```

Transfers `:8001` to the above listening client.
