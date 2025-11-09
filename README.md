# RUNE Artifact Evaluation

This repository contains the source code for the RUNE FaaS system described in the Middleware'25 paper *A Hybrid Runtime for Function-as-a-Service at the Edge*.  

RUNE is implemented as `fecore`, which is the core runtime for the FaaSEdge project at Georgia Tech. This document focuses on getting RUNE up-and-running for the purpose of artifact evaluation. 
The main README for the fecore/RUNE project can be found in [README_fecore.md](README_fecore.md).

## Getting Started

The instructions and setup described in this document have been tested on Cloudlab c220g1 machines. 
The fastest way to evaluate RUNE is to use our pre-built Cloudlab image:
- You will need to register for an account at [Cloudlab](https://www.cloudlab.us)
- Once you have registered, visit the profile page for our [RUNE-ae Profile](https://www.cloudlab.us/p/fecore/RUNE-ae) 
- Select the `Instantiate` button
- You can evaluate RUNE using a single node
- Select a project to attach this experiment to (if you do not already have a project to use, one can be created by selecting your username in the top-right corner and then "Start/Join Project")
- Select "Next" and then "Finish" to launch a new experiment

Cloudlab will provision a bare metal c220g1 machine and load our pre-built Linux image that contains most dependencies needed to run RUNE.

After the machine has been brought online, SSH into it and change directory to `/project/RUNE`. You should find the files needed to complete the setup in this directory. **NOTE**: If you do not see any files in this directory, or if you are not using our Cloudlab image, you will need to manually download the [latest release of RUNE](https://github.com/gt-epl/fecore/releases/download/RUNE-ae-v1.0/RUNE-ae-v1.0.tgz) and run the `SETUP.sh` script found within.

Run the `SETUP.sh` script to finish installation of remaining of dependencies and configs.

Note that you can also set up an environment manually by following instructions in [README_fecore.md](README_fecore.md) and [SETUP.md](docs/SETUP.md). The provided `SETUP.sh` script can also be readily adapted to other Ubuntu 20.04 LTS installations.

## Evaluating RUNE

You will need at least two console sessions to evaluate RUNE. We recommend using a program like `tmux` or `screen` to split the console into two halves so that you can run commands and view output from RUNE at the same time.  

In the first console session, start the RUNE server with the following commands:
- `sudo su -`
- `cd /usr/local/bin`
- `secret_mount_path=/var/lib/fecore/secrets fecore provider`

In the second console session, deploy example functions with the following commands:
```
cd /mnt/faasedge/bin/

faas-cli -g 10.62.0.1:8081 deploy --image achgt/thumbnailer-n:latest --name thumbnailer-n --label ctrType=native
faas-cli -g 10.62.0.1:8081 deploy --image=thumbnailer-w --name thumbnailer-w --label ctrType=wasm
faas-cli -g 10.62.0.1:8081 deploy --image hybrid --name thumbnailer-h --label ctrType=hybrid --label sandboxes=thumbnailer-n,thumbnailer-w

faas-cli -g 10.62.0.1:8081 deploy --image achgt/encrypt-p:latest --name encrypt-n --label ctrType=native
faas-cli -g 10.62.0.1:8081 deploy --image=encrypt-w --name encrypt-w --label ctrType=wasm
faas-cli -g 10.62.0.1:8081 deploy --image hybrid --name encrypt-h --label ctrType=hybrid --label sandboxes=encrypt-n,encrypt-w

faas-cli -g 10.62.0.1:8081 deploy --image achgt/compression-n:latest --name compression-n --label ctrType=native
faas-cli -g 10.62.0.1:8081 deploy --image=compression-w --name compression-w --label ctrType=wasm
faas-cli -g 10.62.0.1:8081 deploy --image hybrid --name compression-h --label ctrType=hybrid --label sandboxes=compression-n,compression-w
```

The above commands will deploy Native, WASM, and Hybrid versions of three example functions described in the RUNE paper (*Thumbnailer*, *Encrypt*, and *Compression*). The Native versions are available as pre-built images via [DockerHub](https://hub.docker.com/u/achgt) and the pre-built WASM versions are included in the [latest release of RUNE](https://github.com/gt-epl/fecore/releases/download/RUNE-ae-v1.0/RUNE-ae-v1.0.tgz) under the `examples` directory.  

You can test RUNE's hybrid container capability by invoking a hybrid function deployment via `curl`, e.g.: `curl -vk http://10.62.0.1:8081/function/compression-h`

The example functions have a default policy that specifies using a WebAssembly (wasm) container for cold starts and a native container for warm starts. The first time you invoke a hybrid function with `curl` you should see output similar to the following:

```
*   Trying 10.62.0.1:8081...
* Connected to 10.62.0.1 (10.62.0.1) port 8081 (#0)
> GET /function/compression-h HTTP/1.1
> Host: 10.62.0.1:8081
> User-Agent: curl/7.74.0
> Accept: */*
>
* Mark bundle as not supporting multiuse
< HTTP/1.1 200 OK
< Container-Name: compression-w_ebfaef91-6224-438a-901a-9c2d3b10e9f8_w
< Container-Type: hybrid
< Content-Type: text/plain
< Invocation-Elapsed: 272
< Misc-Stats: 27.773000,18.215000,373.543000,108.675000
< Request-Id: compression-h_bd842ef8-8ea1-4118-a96c-2898d6b139b3
< Runw-Setup: 1
< Server: runw
< Setup-Time: 23
< Startup-Type: cold
< Date: Sat, 18 Oct 2025 06:48:01 GMT
< Content-Length: 9
<
Success
* Connection #0 to host 10.62.0.1 left intact
```

Two important fields to note in the output are `Container-Name` and `Startup-Type`. Here we see the container name starts with `compression-w`, indicating a WebAssembly container was used. The `Startup-Type` is `cold`.

If you run the curl command again, you should see output similar to the following:

```
*   Trying 10.62.0.1:8081...
* Connected to 10.62.0.1 (10.62.0.1) port 8081 (#0)
> GET /function/compression-h HTTP/1.1
> Host: 10.62.0.1:8081
> User-Agent: curl/7.74.0
> Accept: */*
>
* Mark bundle as not supporting multiuse
< HTTP/1.1 200 OK
< Container-Name: compression-n_b6015450-6462-44aa-aa41-c92bec5b7ce8_n
< Container-Type: hybrid
< Content-Length: 8
< Content-Type: text/plain; charset=utf-8
< Date: Sat, 18 Oct 2025 06:48:03 GMT
< Invocation-Elapsed: 157
< Request-Id: compression-h_47d43fad-c660-4e2d-8edb-cc3f1d878520
< Setup-Time: 0
< Startup-Type: warm
<
1227561
* Connection #0 to host 10.62.0.1 left intact
```

In this second output we see the `Container-Name` now begins with `compression-n`, indicating a native container, and the `Startup-Type` is now `warm`. 

The warm native container will be destroyed after 10 seconds of inactivity, necessitating another cold start. The server output provides detailed information such as how each container was spawned, how much time was required for setup and execution, and when the idle container was deleted.

When you are finished evaluating RUNE, pressing `Ctrl+c` in the console session running `fecore` should clean up any running containers and perform a graceful shutdown.
