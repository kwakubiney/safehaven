# safehaven

* Architecture
  ![architecture](https://github.com/kwakubiney/safehaven/assets/71296367/7a637a3f-337d-4e44-a793-4aa01049d191)

## How does it work
Checkout my blog [post](https://kwakubiney.github.io/posts/UDP-Tunneling-With-Safehaven/) for implementation details.

## Demo
[Click here to watch demo](https://www.youtube.com/watch?v=BJcXyx5ae1Ac)

## How to use?
* Only available on Linux
```
Usage:
  -d string
        private network destination (default "10.108.0.2")
  -g    global
        routes all traffic to tunnel server
  -l string
        local address
  -s string
        remote server address (default "138.197.32.138")
  -srv
        server mode
  -tc string
        client tun device ip (default "192.168.1.100/24")
  -tname string
        tunname (default "tun0")
  -ts string
        server tun device ip (default "192.168.1.102/24")
```

* Build the project.
* Run using appropriate flags on client.
* Run on server in `server mode` and also run `sysctl -w net.ipv4.ip_forward=1` on server to allow IP forwarding.
* `ping` destination address from client after setup & you should receive echo replies back.

`NB`: Your server must know how to reach the private network else your packet will get lost in the sauce on the server.
