# thumbnailer x86/js

```
LATEST BUILD:
2024-02-13
thumbnailer.js md5=a0c10bcd66f27dd4993e8f2cdf7ce1f2
```

Image thumbnail creation in JavaScript. Takes as input a PNG image and outputs a thumbnail of that image in PNG format.

Adapted from the [Serverlessbench](https://github.com/spcl/serverless-benchmarks/tree/master/benchmarks/200.multimedia/210.thumbnailer/nodejs) repo.

This function includes a built-in webserver (i.e., no external loader is needed).

## Build

No build is needed, but some dependencies need to be installed via npm (see `package.json`)

## Run

`node thumbnailer.js` will bind to port 8080 and create a thumbnail from `input.png` image when it receives a GET request.

## Container Creation

Create the container:
```
sudo buildah build --cgroupns private --ipc private --network private --pid private --userns container --uts private --isolation oci --cap-drop cap_net_admin --security-opt seccomp=/usr/share/containers/seccomp.json -t thumbnailer-j .
```
Test the container locally/outside fecore:
```
sudo ctr run -d --net-host localhost/thumbnailer-j:latest thumbnailer-j
```

## Running with fecore

1. Start fecore
2. Deploy thumbnailer-j container image
```
faas-cli -g 10.62.0.1:8081 deploy --image url.of.container.registry/thumbnailer-j:latest --name thumbnailer-j
```

3. Invoke the function:
```
curl http://10.62.0.1:8081/function/thumbnailer-j
```
