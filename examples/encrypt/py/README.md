# encrypt x86/python

```
LATEST BUILD:
2024-02-13
encrypt.py md5=46aa17741f58b43f11c2afb3c6e04b70
```

AES 128-bit CTR encryption in Python using `pyaes` library. Encrypts the string `hello world` 10,000 times.

Adapted from the [Serverless-FaaS-Workbench](https://github.com/ddps-lab/serverless-faas-workbench/tree/master/openwhisk/cpu-memory/pyaes) repo.

This function includes a built-in webserver (i.e., no external loader is needed).

## Build

No build is needed, but `pyaes` must be installed.

## Run

`encrypt.py` will bind to port 8080 and perform encryption when it receives a GET request.

## Container Creation

Create the container:
```
sudo buildah build --cgroupns private --ipc private --network private --pid private --userns container --uts private --isolation oci --cap-drop cap_net_admin --security-opt seccomp=/usr/share/containers/seccomp.json -t encrypt-p .
```

Test the container locally/outside fecore:
```
sudo ctr run -d --net-host localhost/encrypt-p:latest encrypt-p
```

## Running with fecore

1. Start fecore
2. Deploy encrypt-p container image
```
faas-cli -g 10.62.0.1:8081 deploy --image url.to.container.registry/encrypt-p:latest --name encrypt-p
```

3. Invoke the function:
```
curl http://10.62.0.1:8081/function/encrypt-p
```
